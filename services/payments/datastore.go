package payments

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/aws/smithy-go"
	"github.com/brave-intl/bat-go/libs/logging"
	appaws "github.com/brave-intl/bat-go/libs/nitro/aws"
	"github.com/shopspring/decimal"
	"golang.org/x/exp/slices"

	"github.com/amazon-ion/ion-go/ion"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmsTypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/aws/aws-sdk-go-v2/service/qldbsession"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/awslabs/amazon-qldb-driver-go/v3/qldbdriver"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/google/uuid"
)

type AuthenticatedPaymentState struct {
	PaymentDetails
	Status         PaymentStatus
	Authorizations []PaymentAuthorization `json:"authorizations"`
	DryRun         *string                `json:"dryRun"` // determines dry-run
	LastError      *PaymentError
	documentID     string
}

type PaymentDetails struct {
	Amount    *ion.Decimal `json:"amount" valid:"required"`
	To        string       `json:"to,omitempty" valid:"required"`
	From      string       `json:"from,omitempty" valid:"required"`
	Custodian string       `json:"custodian,omitempty" valid:"in(uphold|gemini|bitflyer)"`
	PayoutID  string       `json:"payoutId" valid:"required"`
	Currency  string       `json:"currency"`
}

// PaymentAuthorization represents a single authorization from a payment authorizer indicating that
// the payout represented by a document ID should be processed
type PaymentAuthorization struct {
	KeyID      string `json:"keyId" valid:"required"`
	DocumentID string `json:"documentId" valid:"required"`
}

// qldbPaymentTransitionHistoryEntryBlockAddress defines blockAddress data for
// qldbPaymentTransitionHistoryEntry.
type qldbPaymentTransitionHistoryEntryBlockAddress struct {
	StrandID   string `ion:"strandId"`
	SequenceNo int64  `ion:"sequenceNo"`
}

// qldbPaymentTransitionHistoryEntryHash defines hash for qldbPaymentTransitionHistoryEntry.
type qldbPaymentTransitionHistoryEntryHash string

// qldbPaymentTransitionHistoryEntrySignature defines signature for
// qldbPaymentTransitionHistoryEntry.
type qldbPaymentTransitionHistoryEntrySignature []byte

// PaymentState defines data for qldbPaymentTransitionHistoryEntry.
type PaymentState struct {
	// Serialized AuthenticatedPaymentState. Should only ever be access via GetSafePaymentState,
	// which does all of the needed validation of the state
	unsafePaymentState []byte     `ion:"data"`
	Signature          []byte     `ion:"signature"`
	ID                 *uuid.UUID `ion:"idempotencyKey"`
}

// qldbPaymentTransitionHistoryEntryMetadata defines metadata for qldbPaymentTransitionHistoryEntry
type qldbPaymentTransitionHistoryEntryMetadata struct {
	ID      string    `ion:"id"`
	TxID    string    `ion:"txId"`
	TxTime  time.Time `ion:"txTime"`
	Version int64     `ion:"version"`
}

// qldbPaymentTransitionHistoryEntry defines top level entry for a QLDB transaction.
type qldbPaymentTransitionHistoryEntry struct {
	BlockAddress qldbPaymentTransitionHistoryEntryBlockAddress `ion:"blockAddress"`
	Hash         qldbPaymentTransitionHistoryEntryHash         `ion:"hash"`
	Data         PaymentState                                  `ion:"data"`
	Metadata     qldbPaymentTransitionHistoryEntryMetadata     `ion:"metadata"`
}

type qldbDocumentIDResult struct {
	documentID string `ion:"documentId"`
}

func (e *qldbPaymentTransitionHistoryEntry) toTransaction() (*AuthenticatedPaymentState, error) {
	var txn AuthenticatedPaymentState
	err := json.Unmarshal(e.Data.unsafePaymentState, &txn)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to unmarshal record data for conversion from qldbPaymentTransitionHistoryEntry to Transaction: %w",
			err,
		)
	}
	return &txn, nil
}

func (p *PaymentState) toAuthenticatedPaymentState() (*AuthenticatedPaymentState, error) {
	var txn AuthenticatedPaymentState
	err := json.Unmarshal(p.unsafePaymentState, &txn)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to unmarshal record data for conversion from qldbPaymentTransitionHistoryEntry to Transaction: %w",
			err,
		)
	}
	return &txn, nil
}

// PaymentError is an error used to communicate whether an error is temporary.
type PaymentError struct {
	OriginalError  error
	FailureMessage string
	Temporary      bool
}

// Error makes ProcessingError an error
func (e PaymentError) Error() string {
	msg := fmt.Sprintf("error: %s", e.FailureMessage)
	if e.Cause() != nil {
		msg = fmt.Sprintf("%s: %s", msg, e.Cause())
	}
	return msg
}

// Cause implements Cause for error
func (e PaymentError) Cause() error {
	return e.OriginalError
}

// Unwrap implements Unwrap for error
func (e PaymentError) Unwrap() error {
	return e.OriginalError
}

// ProcessingErrorFromError - given an error turn it into a processing error
func ProcessingErrorFromError(cause error, isTemporary bool) error {
	return &PaymentError{
		OriginalError:  cause,
		FailureMessage: cause.Error(),
		Temporary:      isTemporary,
	}
}

// GenerateIdempotencyKey returns a UUID v5 ID if the ID on the Transaction matches its expected value. Otherwise, it returns
// an error.
func (p *PaymentState) GenerateIdempotencyKey(namespace uuid.UUID) (*uuid.UUID, error) {
	authenticatedState, err := p.toAuthenticatedPaymentState()
	if err != nil {
		return nil, err
	}
	generatedIdempotencyKey := authenticatedState.generateIdempotencyKey(namespace)
	if generatedIdempotencyKey != *p.ID {
		return nil, fmt.Errorf("ID does not match transaction fields: have %s, want %s", *p.ID, generatedIdempotencyKey)
	}

	return p.ID, nil
}

// SetIdempotencyKey assigns a UUID v5 value to PaymentState.ID.
func (p *PaymentState) SetIdempotencyKey(namespace uuid.UUID) error {
	authenticatedPaymentState, err := p.toAuthenticatedPaymentState()
	if err != nil {
		return err
	}
	generatedIdempotencyKey := authenticatedPaymentState.generateIdempotencyKey(namespace)
	p.ID = &generatedIdempotencyKey
	return nil
}

var (
	prepareFailure = "prepare"
	submitFailure  = "submit"
)

// SignTransaction - perform KMS signing of the transaction, return publicKey and signature in hex
// string.
func (a *AuthenticatedPaymentState) SignTransaction(
	ctx context.Context,
	kmsClient wrappedKMSClient,
	keyID string,
) (string, string, error) {
	pubkeyOutput, err := kmsClient.GetPublicKey(ctx, &kms.GetPublicKeyInput{
		KeyId: &keyID,
	})

	if err != nil {
		return "", "", fmt.Errorf("Failed to get public key: %w", err)
	}

	marshaled, err := ion.MarshalBinary(a)
	if err != nil {
		return "", "", fmt.Errorf("Ion marshal failed: %w", err)
	}

	signingOutput, err := kmsClient.Sign(ctx, &kms.SignInput{
		KeyId:            &keyID,
		Message:          marshaled,
		SigningAlgorithm: kmsTypes.SigningAlgorithmSpecEcdsaSha256,
	})

	if err != nil {
		return "", "", fmt.Errorf("Failed to sign transaction: %w", err)
	}

	return hex.EncodeToString(pubkeyOutput.PublicKey), hex.EncodeToString(signingOutput.Signature), nil
}

// MarshalJSON - custom marshaling of transaction type.
func (a *AuthenticatedPaymentState) MarshalJSON() ([]byte, error) {
	type Alias AuthenticatedPaymentState
	return json.Marshal(&struct {
		Amount *decimal.Decimal `json:"amount"`
		*Alias
	}{
		Amount: fromIonDecimal(a.Amount),
		Alias:  (*Alias)(a),
	})
}

// UnmarshalJSON - custom unmarshal of transaction type.
func (a *AuthenticatedPaymentState) UnmarshalJSON(data []byte) error {
	type Alias AuthenticatedPaymentState
	aux := &struct {
		Amount *decimal.Decimal `json:"amount"`
		*Alias
	}{
		Alias: (*Alias)(a),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return fmt.Errorf("failed to unmarshal transaction: %w", err)
	}
	if aux.Amount == nil {
		return fmt.Errorf("missing required transaction value: Amount")
	}
	parsedAmount, err := ion.ParseDecimal(aux.Amount.String())
	if err != nil {
		return fmt.Errorf("failed to parse transaction Amount into ion decimal: %w", err)
	}
	a.Amount = parsedAmount
	return nil
}

func (p *AuthenticatedPaymentState) generateIdempotencyKey(namespace uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(
		namespace,
		[]byte(fmt.Sprintf(
			"%s%s%s%s%s%s",
			p.To,
			p.From,
			p.Currency,
			p.Amount,
			p.Custodian,
			p.PayoutID,
		)),
	)
}

func (t *PaymentState) getIdempotencyKey() *uuid.UUID {
	return t.ID
}

func (t *AuthenticatedPaymentState) nextStateValid(nextState PaymentStatus) bool {
	if t.Status == nextState {
		return true
	}
	// New transaction state should be present in the list of valid next states for the current
	// state.
	if !slices.Contains(t.Status.GetValidTransitions(), nextState) {
		return false
	}
	return true
}

func fromIonDecimal(v *ion.Decimal) *decimal.Decimal {
	value, exp := v.CoEx()
	resp := decimal.NewFromBigInt(value, exp)
	return &resp
}

// ErrNotConfiguredYet - service not fully configured.
var ErrNotConfiguredYet = errors.New("not yet configured")

func (s *Service) configureDatastore(ctx context.Context) error {
	driver, err := newQLDBDatastore(ctx)
	if err != nil {
		if errors.Is(err, ErrNotConfiguredYet) {
			// will eventually get configured
			return nil
		}
		return fmt.Errorf("failed to create new qldb datastore: %w", err)
	}
	s.datastore = driver
	return s.setupLedger(ctx)
}

const tableAlreadyCreatedCode = "412"

func (s *Service) setupLedger(ctx context.Context) error {
	logger := logging.Logger(ctx, "payments.setupLedger")
	// create the tables needed in the ledger
	_, err := s.datastore.Execute(context.Background(), func(txn qldbdriver.Transaction) (interface{}, error) {
		var (
			ae smithy.APIError
			ok bool
		)
		_, err := txn.Execute("CREATE TABLE transactions")
		if err != nil {
			logger.Warn().Err(err).Msg("error creating transactions table")
			if errors.As(err, &ae) {
				logger.Warn().Err(err).Str("code", ae.ErrorCode()).Msg("api error creating transactions table")
				if ae.ErrorCode() == tableAlreadyCreatedCode {
					// table has already been created
					ok = true
				}
			}
			if !ok {
				return nil, fmt.Errorf("failed to create transactions table due to: %w", err)
			}
		}

		_, err = txn.Execute("CREATE INDEX ON transactions (idempotencyKey)")
		if err != nil {
			return nil, err
		}

		ok = false
		_, err = txn.Execute("CREATE TABLE authorizations")
		if err != nil {
			logger.Warn().Err(err).Msg("error creating authorizationss table")
			if errors.As(err, &ae) {
				logger.Warn().Err(err).Str("code", ae.ErrorCode()).Msg("api error creating authorizations table")
				if ae.ErrorCode() == tableAlreadyCreatedCode {
					// table has already been created
					ok = true
				}
			}
			if !ok {
				return nil, fmt.Errorf("failed to create transactions table due to: %w", err)
			}
		}
		return nil, nil
	})
	if err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}
	return nil
}

// PrepareTransaction - perform a qldb insertion on the transaction.
func (s *Service) PrepareTransaction(
	ctx context.Context,
	ID *uuid.UUID,
	transaction *AuthenticatedPaymentState,
) (*AuthenticatedPaymentState, error) {
	stateMachine, err := StateMachineFromTransaction(ID, transaction, s)
	if err != nil {
		return nil, fmt.Errorf("failed to create state machine: %w", err)
	}
	txn, err := populateInitialTransaction(ctx, stateMachine)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare transaction: %w", err)
	}
	return txn, nil
}

// newQLDBDatastore - create a new qldbDatastore.
func newQLDBDatastore(ctx context.Context) (*qldbdriver.QLDBDriver, error) {
	logger := logging.Logger(ctx, "payments.newQLDBDatastore")

	if !isQLDBReady(ctx) {
		return nil, ErrNotConfiguredYet
	}

	egressProxyAddr, ok := ctx.Value(appctx.EgressProxyAddrCTXKey).(string)
	if !ok {
		return nil, fmt.Errorf("failed to get egress proxy for qldb")
	}

	// decrypt the aws region
	region, ok := ctx.Value(appctx.AWSRegionCTXKey).(string)
	if !ok {
		err := errors.New("empty aws region")
		logger.Error().Err(err).Str("region", region).Msg("aws region")
		return nil, err
	}

	// qldb role arn
	qldbRoleArn, ok := ctx.Value(appctx.PaymentsQLDBRoleArnCTXKey).(string)
	if !ok {
		err := errors.New("empty qldb role arn")
		logger.Error().Err(err).Str("qldbRoleArn", qldbRoleArn).Msg("qldb role arn empty")
		return nil, err
	}

	// qldb ledger name
	qldbLedgerName, ok := ctx.Value(appctx.PaymentsQLDBLedgerNameCTXKey).(string)
	if !ok {
		err := errors.New("empty qldb ledger name")
		logger.Error().Err(err).Str("qldbLedgerName", qldbLedgerName).Msg("qldb ledger name empty")
		return nil, err
	}

	logger.Info().
		Str("egress", egressProxyAddr).
		Str("region", region).
		Str("qldbRoleArn", qldbRoleArn).
		Str("qldbLedgerName", qldbLedgerName).
		Msg("qldb details")

	awsCfg, err := appaws.NewAWSConfig(ctx, egressProxyAddr, region)
	if err != nil {
		logger.Error().Err(err).Str("region", region).Msg("aws config failed")
		return nil, fmt.Errorf("failed to create aws config: %w", err)
	}

	// assume correct role for qldb access
	creds := stscreds.NewAssumeRoleProvider(sts.NewFromConfig(awsCfg), qldbRoleArn)
	awsCfg.Credentials = aws.NewCredentialsCache(creds)

	client := qldbsession.NewFromConfig(awsCfg)
	// create our qldb driver
	driver, err := qldbdriver.New(
		qldbLedgerName, // the ledger to attach to
		client,         // the qldb session
		func(options *qldbdriver.DriverOptions) {
			// debug mode?
			debug, err := appctx.GetBoolFromContext(ctx, appctx.DebugLoggingCTXKey)
			if err == nil && debug {
				options.LoggerVerbosity = qldbdriver.LogDebug
			} else {
				// default to info
				options.LoggerVerbosity = qldbdriver.LogInfo
			}
		})
	if err != nil {
		return nil, fmt.Errorf("failed to setup the qldb driver: %w", err)
	}
	// setup a retry policy
	// Configuring an exponential backoff strategy with base of 20 milliseconds
	retryPolicy2 := qldbdriver.RetryPolicy{
		MaxRetryLimit: 2,
		Backoff:       qldbdriver.ExponentialBackoffStrategy{SleepBase: 20, SleepCap: 4000}}

	// Overrides the retry policy set by the driver instance
	driver.SetRetryPolicy(retryPolicy2)

	return driver, nil
}

const (
	// StatePrepared - transaction prepared state
	StatePrepared = "prepared"
	// StateSubmitted - transaction prepared state
	StateSubmitted = "submitted"
)

// InsertTransaction - perform a qldb insertion on the transactions.
func (s Service) InsertTransaction(
	ctx context.Context,
	state *AuthenticatedPaymentState,
) (AuthenticatedPaymentState, error) {
	enrichedTransaction, err := s.datastore.Execute(context.Background(), func(
		txn qldbdriver.Transaction,
	) (interface{}, error) {
		// for all of the transactions load up a check to see if this transaction has already
		// existed or not, then perform the insertion of the records.
		resp := AuthenticatedPaymentState{}

		namespace := ctx.Value(serviceNamespaceContextKey{}).(uuid.UUID)
		id := state.generateIdempotencyKey(namespace)

		// Check if a document with this idempotencyKey exists
		result, err := txn.Execute("SELECT * FROM transactions WHERE idempotencyKey = ?", id)
		if err != nil {
			return nil, fmt.Errorf("failed to insert tx: %s due to: %w", id, err)
		}
		// Check if there are any results. There should not be. If there are, we should not insert.
		if !result.Next(txn) {
			// set transaction state to prepared
			state.Status = StatePrepared
			// insert the transaction
			insert_result, err := txn.Execute("INSERT INTO transactions ?", state)
			if err != nil {
				return nil, fmt.Errorf("failed to insert tx: %s due to: %w", id, err)
			}
			ionBinary := insert_result.GetCurrentData()

			temp := new(qldbDocumentIDResult)
			err = ion.Unmarshal(ionBinary, temp)
			if err != nil {
				return nil, err
			}
			resp.documentID = temp.documentID
		}

		return resp, fmt.Errorf("failed to insert transaction because id already exists: %s", id)
	})
	if err != nil {
		return AuthenticatedPaymentState{}, fmt.Errorf("failed to insert transaction: %w", err)
	}
	return enrichedTransaction.(AuthenticatedPaymentState), nil
}

// UpdateTransactionsState - Change transaction state.
//func (s Service) UpdateTransactionsState(
//	ctx context.Context,
//	state string,
//	transactions ...AuthenticatedPaymentState,
//) error {
//	_, err := s.datastore.Execute(
//		context.Background(),
//		func(txn qldbdriver.Transaction) (interface{}, error) {
//			// for all of the transactions load up a check to see if this transaction has already
//			// existed or not, then perform the insertion of the records.
//			for _, transaction := range transactions {
//				// update the transaction state, ignoring the returned document ID
//				_, err := txn.Execute(
//					"UPDATE transactions BY d_id SET state = ? WHERE d_id = ?",
//					state,
//					transaction.documentID,
//				)
//				if err != nil {
//					return nil, fmt.Errorf(
//						"failed to update state for document: %s due to: %w",
//						transaction.documentID,
//						err,
//					)
//				}
//			}
//			return nil, nil
//		})
//	if err != nil {
//		return fmt.Errorf("failed to update transactions: %w", err)
//	}
//	return nil
//}

// AuthorizeTransaction - Add an Authorization for the Transaction and attempt to Drive
// the Transaction forward. NOTE: This function assumes that the http signature has been
// verified before running. This is achieved in the SubmitHandler middleware.
func (s *Service) AuthorizeTransaction(
	ctx context.Context,
	keyID string,
	transaction AuthenticatedPaymentState,
) error {
	fetchedTxn, idempotencyKey, err := s.GetTransactionFromDocumentID(ctx, transaction.documentID)
	if err != nil {
		return fmt.Errorf(
			"failed to get transaction with idempotencyKey %s by document ID %s: %w",
			idempotencyKey,
			transaction.documentID,
			err,
		)
	}
	auth := PaymentAuthorization{
		KeyID:      keyID,
		DocumentID: transaction.documentID,
	}
	keyHasNotYetSigned := true
	for _, authorization := range fetchedTxn.Authorizations {
		if authorization.KeyID == auth.KeyID {
			keyHasNotYetSigned = false
		}
	}
	if !keyHasNotYetSigned {
		return fmt.Errorf("key %s has already signed document %s", auth.KeyID, fetchedTxn.documentID)
	}
	fetchedTxn.Authorizations = append(fetchedTxn.Authorizations, auth)
	writtenTxn, err := WriteTransaction(
		ctx,
		s.datastore,
		s.sdkClient,
		s.kmsSigningClient,
		s.kmsSigningKeyID,
		fetchedTxn,
	)
	if err != nil {
		return fmt.Errorf("failed to update transaction: %w", err)
	}
	stateMachine, err := StateMachineFromTransaction(idempotencyKey, writtenTxn, s)
	if err != nil {
		return fmt.Errorf("failed to create stateMachine: %w", err)
	}
	_, err = Drive(ctx, stateMachine)
	if err != nil {
		// Insufficient authorizations is an expected state. Treat it as such.
		var insufficientAuthorizations *InsufficientAuthorizationsError
		if errors.As(err, &insufficientAuthorizations) {
			return nil
		}
		return fmt.Errorf("failed to progress transaction: %w", err)
	}
	// If the above call to Drive succeeds without giving insufficientAuthorizations,
	// it's time to kick off payment. @TODO: Needs to be async, but for dry-run we
	// can leave it synchronous.
	_, err = Drive(ctx, stateMachine)
	if err != nil {
		return fmt.Errorf("failed to progress transaction: %w", err)
	}

	return nil
}

// GetTransactionFromDocumentID - get the transaction data from the document ID in qldb.
func (s *Service) GetTransactionFromDocumentID(
	ctx context.Context,
	documentID string,
) (*AuthenticatedPaymentState, *uuid.UUID, error) {
	paymentStateInterface, err := s.datastore.Execute(
		context.Background(),
		func(txn qldbdriver.Transaction) (interface{}, error) {
			resp := new(PaymentState)

			result, err := txn.Execute(
				"SELECT data.*, d_id FROM transactions BY d_id WHERE d_id = ?",
				documentID,
			)
			if err != nil {
				return nil, fmt.Errorf("failed to get tx: %s due to: %w", documentID, err)
			}
			// Check if there are any results
			if result.Next(txn) {
				// unmarshal enriched version
				err := ion.Unmarshal(result.GetCurrentData(), resp)
				if err != nil {
					return nil, fmt.Errorf("failed to unmarshal tx: %s due to: %w", documentID, err)
				}
			}

			return resp, nil
		},
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get transactions: %w", err)
	}
	paymentState := paymentStateInterface.(*PaymentState)
	authenticatedState, err := paymentState.toAuthenticatedPaymentState()
	if err != nil {
		return nil, nil, err
	}
	authenticatedState.documentID = documentID
	return authenticatedState, paymentState.ID, nil
}

// getQLDBObject returns the latest version of an entry for a given ID after doing all requisite
// validation.
func getQLDBObject(
	ctx context.Context,
	qldbTransactionDriver wrappedQldbTxnAPI,
	sdkClient wrappedQldbSDKClient,
	kmsSigningClient wrappedKMSClient,
	documentID string,
) (*qldbPaymentTransitionHistoryEntry, error) {
	valid, result, err := transactionHistoryIsValid(
		ctx,
		qldbTransactionDriver,
		kmsSigningClient,
		documentID,
	)
	if err != nil || !valid {
		return nil, fmt.Errorf("failed to validate transition history: %w", err)
	}
	// If no record was found, return nothing
	if result == nil {
		return nil, &QLDBReocrdNotFoundError{}
	}
	merkleValid, err := revisionValidInTree(ctx, sdkClient, result)
	if err != nil {
		return nil, fmt.Errorf("failed to verify Merkle tree: %w", err)
	}
	if !merkleValid {
		return nil, fmt.Errorf("invalid Merkle tree for record: %#v", result)
	}
	return result, nil
}

// GetTransactionByIdempotencyKey returns the latest version of a record from QLDB if it exists, after doing all requisite validation.
func GetTransactionByIdempotencyKey(
	ctx context.Context,
	datastore wrappedQldbDriverAPI,
	sdkClient wrappedQldbSDKClient,
	kmsSigningClient wrappedKMSClient,
	idempotencyKey *uuid.UUID,
) (*AuthenticatedPaymentState, error) {
	stateInterface, err := datastore.Execute(ctx, func(txn qldbdriver.Transaction) (interface{}, error) {
		resp := new(AuthenticatedPaymentState)

		// Check if a document with this idempotencyKey exists
		result, err := txn.Execute(
			"SELECT data.*, d_id FROM transactions BY d_id WHERE idempotencyKey = ?",
			idempotencyKey,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to get by id: %s due to: %w", idempotencyKey, err)
		}
		if result.Next(txn) {
			// unmarshal enriched version
			err := ion.Unmarshal(result.GetCurrentData(), resp)
			if err != nil {
				return nil, fmt.Errorf("failed to unmarshal tx: %s due to: %w", idempotencyKey, err)
			}
		}

		return resp, nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query QLDB: %w", err)
	}
	assertedState, ok := stateInterface.(*AuthenticatedPaymentState)
	if !ok {
		return nil, fmt.Errorf("database response was the wrong type: %#v", stateInterface)
	}
	return assertedState, nil
}

// getTransactionHistory returns a slice of entries representing the entire state history
// for a given id.
func getTransactionHistory(
	txn wrappedQldbTxnAPI,
	documentID string,
) ([]qldbPaymentTransitionHistoryEntry, error) {
	result, err := txn.Execute(
		"SELECT * FROM history(transactions) AS h WHERE h.metadata.id = ?",
		documentID,
	)
	if err != nil {
		return nil, fmt.Errorf("QLDB transaction failed: %w", err)
	}
	var collectedData []qldbPaymentTransitionHistoryEntry
	for result.Next(txn) {
		var data qldbPaymentTransitionHistoryEntry
		err := ion.Unmarshal(result.GetCurrentData(), &data)
		if err != nil {
			return nil, fmt.Errorf("ion unmarshal failed: %w", err)
		}
		collectedData = append(collectedData, data)
	}
	return collectedData, nil
}

// WriteTransaction persists an object in a transaction after verifying that its change
// represents a valid state transition.
func WriteTransaction(
	ctx context.Context,
	datastore wrappedQldbDriverAPI,
	sdkClient wrappedQldbSDKClient,
	kmsSigningClient wrappedKMSClient,
	kmsSigningKeyID string,
	transaction *AuthenticatedPaymentState,
) (*AuthenticatedPaymentState, error) {
	_, err := datastore.Execute(
		ctx,
		func(txn qldbdriver.Transaction) (interface{}, error) {
			// This call will do all necessary record and history validation if they exist for this
			// record
			_, err := getQLDBObject(ctx, txn, sdkClient, kmsSigningClient, transaction.documentID)
			var notFound *QLDBReocrdNotFoundError
			if err != nil {
				if errors.As(err, &notFound) {
					return nil, fmt.Errorf("document does not exist: %w", err)
				}
				return nil, fmt.Errorf("failed to query QLDB: %w", err)
			}

			result, err := json.Marshal(transaction)
			if err != nil {
				return nil, err
			}

			/*publicKey*/
			_, signature, err := transaction.SignTransaction(
				ctx,
				kmsSigningClient,
				kmsSigningKeyID,
			)
			if err != nil {
				return nil, fmt.Errorf("failed to sign transaction: %w", err)
			}

			return txn.Execute(
				"UPDATE transactions BY d_id SET data = ?, signature = ? WHERE d_id = ?",
				result,
				signature,
				transaction.documentID,
			)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("QLDB write execution failed: %w", err)
	}
	return transaction, nil
}
