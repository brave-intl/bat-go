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
	. "github.com/brave-intl/bat-go/libs/payments"
)

// SignTransaction - perform KMS signing of the transaction, return publicKey and signature in hex
// string.
func (s *Service) SignTransaction(
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
	id uuid.UUID,
	transaction *AuthenticatedPaymentState,
) (*AuthenticatedPaymentState, error) {
	stateMachine, err := StateMachineFromTransaction(id, transaction, s)
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
	stateMachine, err := StateMachineFromTransaction(*idempotencyKey, writtenTxn, s)
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
) (*QLDBPaymentTransitionHistoryEntry, error) {
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
	idempotencyKey uuid.UUID,
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
) ([]QLDBPaymentTransitionHistoryEntry, error) {
	result, err := txn.Execute(
		"SELECT * FROM history(transactions) AS h WHERE h.metadata.id = ?",
		documentID,
	)
	if err != nil {
		return nil, fmt.Errorf("QLDB transaction failed: %w", err)
	}
	var collectedData []QLDBPaymentTransitionHistoryEntry
	for result.Next(txn) {
		var data QLDBPaymentTransitionHistoryEntry
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
