package payments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	errorutils "github.com/brave-intl/bat-go/libs/errors"
	"github.com/brave-intl/bat-go/libs/logging"
	appaws "github.com/brave-intl/bat-go/libs/nitro/aws"
	"github.com/shopspring/decimal"

	"github.com/amazon-ion/ion-go/ion"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/qldbsession"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"
	"github.com/awslabs/amazon-qldb-driver-go/v3/qldbdriver"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/google/uuid"
)

// Transaction - the main type explaining a transaction, type used for qldb via ion
type Transaction struct {
	IdempotencyKey      *uuid.UUID   `json:"idempotencyKey,omitempty" ion:"idempotencyKey" valid:"required"`
	Amount              *ion.Decimal `json:"-" ion:"amount" valid:"required"`
	To                  *uuid.UUID   `json:"to,omitempty" ion:"to" valid:"required"`
	From                *uuid.UUID   `json:"from,omitempty" ion:"from" valid:"required"`
	Custodian           string       `json:"custodian,omitempty" ion:"custodian" valid:"in(uphold|gemini|bitflyer)"`
	State               string       `json:"state,omitempty" ion:"state"`
	DocumentID          string       `json:"documentId,omitempty" ion:"id"`
	AttestationDocument string       `json:"attestation,omitempty" ion:"-"`
	Signature           string       `json:"-" ion:"signature"` // KMS signature only enclave can sign
	PublicKey           string       `json:"-" ion:"publicKey"` // KMS signature only enclave can sign
}

// SignTransaction - perform KMS signing of the transaction, return publicKey and signature in hex string
func (t *Transaction) SignTransaction(ctx context.Context) (string, string, error) {
	// TODO: fill in
	return "", "", errorutils.ErrNotImplemented
}

// BuildSigningBytes - the string format that payments will sign over per tx
func (t Transaction) BuildSigningBytes() []byte {
	return []byte(fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s",
		1, t.To, t.Amount.String(), t.IdempotencyKey, t.Custodian, t.DocumentID, t.State))
}

// MarshalJSON - custom marshaling of transaction type
func (t *Transaction) MarshalJSON() ([]byte, error) {
	type Alias Transaction
	return json.Marshal(&struct {
		Amount *decimal.Decimal `json:"amount"`
		*Alias
	}{
		Amount: fromIonDecimal(t.Amount),
		Alias:  (*Alias)(t),
	})
}

// UnmarshalJSON - custom unmarshal of transaction type
func (t *Transaction) UnmarshalJSON(data []byte) error {
	type Alias Transaction
	aux := &struct {
		Amount *decimal.Decimal `json:"amount"`
		*Alias
	}{
		Alias: (*Alias)(t),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	t.Amount = toIonDecimal(aux.Amount)
	return nil
}

func toIonDecimal(v *decimal.Decimal) *ion.Decimal {
	return ion.MustParseDecimal(v.String())
}

func fromIonDecimal(v *ion.Decimal) *decimal.Decimal {
	value, exp := v.CoEx()
	resp := decimal.NewFromBigInt(value, exp)
	return &resp
}

// ErrNotConfiguredYet - service not fully configured
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

func isQLDBReady(ctx context.Context) bool {
	logger := logging.Logger(ctx, "payments.isQLDBReady")
	// decrypt the aws region
	qldbArn, qldbArnOK := ctx.Value(appctx.PaymentsQLDBRoleArnCTXKey).(string)
	// decrypt the aws region
	region, regionOK := ctx.Value(appctx.AWSRegionCTXKey).(string)
	// get proxy address for outbound
	egressAddr, egressOK := ctx.Value(appctx.EgressProxyAddrCTXKey).(string)
	if regionOK && egressOK && qldbArnOK {
		return true
	}
	logger.Warn().
		Str("region", region).
		Str("egressAddr", egressAddr).
		Str("qldbArn", qldbArn).
		Msg("service is not configured to access qldb")
	return false
}

// newQLDBDatastore - create a new qldbDatastore
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

	cfg, err := appaws.NewAWSConfig(ctx, egressProxyAddr, region)
	if err != nil {
		logger.Error().Err(err).Str("region", region).Msg("aws config failed")
		return nil, fmt.Errorf("failed to create aws config: %w", err)
	}
	awsCfg, ok := cfg.(aws.Config)
	if !ok {
		return nil, fmt.Errorf("invalid aws configuration: %w", err)
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

// InsertTransaction - perform a qldb insertion on the transactions
func (s Service) InsertTransaction(ctx context.Context, transaction *Transaction) (Transaction, error) {
	enrichedTransaction, err := s.datastore.Execute(context.Background(), func(txn qldbdriver.Transaction) (interface{}, error) {
		// for all of the transactions load up a check to see if this transaction has already existed
		// or not, then perform the insertion of the records.
		resp := Transaction{}

		// Check if a document with this idempotencyKey exists
		result, err := txn.Execute("SELECT * FROM transactions WHERE idempotencyKey = ?", transaction.IdempotencyKey)
		if err != nil {
			return nil, fmt.Errorf("failed to insert tx: %s due to: %w", transaction.IdempotencyKey, err)
		}
		// Check if there are any results
		if !result.Next(txn) {
			// set transaction state to prepared
			transaction.State = StatePrepared
			// insert the transaction
			_, err = txn.Execute("INSERT INTO transactions ?", transaction)
			if err != nil {
				return nil, fmt.Errorf("failed to insert tx: %s due to: %w", transaction.IdempotencyKey, err)
			}
		}
		// get the document id for the inserted transaction
		result, err = txn.Execute("SELECT data.*, metadata.id FROM _ql_committed_transactions as t WHERE t.data.idempotencyKey = ?", transaction.IdempotencyKey)
		if err != nil {
			return nil, fmt.Errorf("failed to insert tx: %s due to: %w", transaction.IdempotencyKey, err)
		}
		// Check if there are any results
		if result.Next(txn) {

			// get the enriched version of the transaction for the response
			enriched := new(Transaction)
			ionBinary := result.GetCurrentData()

			// unmarshal enriched version
			err := ion.Unmarshal(ionBinary, enriched)
			if err != nil {
				return nil, fmt.Errorf("failed to unmarshal enriched tx: %s due to: %w", transaction.IdempotencyKey, err)
			}
			resp = *enriched
		}

		return resp, nil
	})
	if err != nil {
		return Transaction{}, fmt.Errorf("failed to insert transactions: %w", err)
	}
	return enrichedTransaction.(Transaction), nil
}

// UpdateTransactionsState - Change transaction state
func (s Service) UpdateTransactionsState(ctx context.Context, state string, transactions ...Transaction) error {
	_, err := s.datastore.Execute(context.Background(), func(txn qldbdriver.Transaction) (interface{}, error) {
		// for all of the transactions load up a check to see if this transaction has already existed
		// or not, then perform the insertion of the records.
		for _, transaction := range transactions {
			// Check if a document with this idempotencyKey exists
			result, err := txn.Execute("SELECT * FROM transactions WHERE idempotencyKey = ?", transaction.IdempotencyKey)
			if err != nil {
				return nil, fmt.Errorf("failed to update tx: %s due to: %w", transaction.IdempotencyKey, err)
			}
			// Check if there are any results
			if result.Next(txn) {
				// update the transaction state
				_, err = txn.Execute("UPDATE transactions SET state = ? WHERE idempotencyKey = ?", state, transaction.IdempotencyKey)
				if err != nil {
					return nil, fmt.Errorf("failed to insert tx: %s due to: %w", transaction.IdempotencyKey, err)
				}
			}
		}
		return nil, nil
	})
	if err != nil {
		return fmt.Errorf("failed to update transactions: %w", err)
	}
	return nil
}

// AuthorizeTransaction - Add an Authorization for the Transaction
func (s Service) AuthorizeTransaction(ctx context.Context, keyID string, transaction Transaction) error {
	_, err := s.datastore.Execute(context.Background(), func(txn qldbdriver.Transaction) (interface{}, error) {
		// for all of the transactions load up a check to see if this transaction has already existed
		// or not, then perform the insertion of the records.
		auth := map[string]string{
			"keyId":      keyID,
			"documentId": transaction.DocumentID,
		}
		_, err := txn.Execute("INSERT INTO authorizations ?", auth)
		if err != nil {
			return nil, fmt.Errorf("failed to insert tx authorization: %+v due to: %w", auth, err)
		}
		return nil, nil
	})
	if err != nil {
		return fmt.Errorf("failed to update transactions: %w", err)
	}
	return nil
}

// GetTransactionFromDocID - get the transaction data from the document ID in qldb
func (s Service) GetTransactionFromDocID(ctx context.Context, docID string) (*Transaction, error) {
	transaction, err := s.datastore.Execute(context.Background(), func(txn qldbdriver.Transaction) (interface{}, error) {
		resp := new(Transaction)

		result, err := txn.Execute("SELECT data.*,metadata.id FROM _ql_committed_transactions WHERE metadata.id = ?", docID)
		if err != nil {
			return nil, fmt.Errorf("failed to get tx: %s due to: %w", docID, err)
		}
		// Check if there are any results
		if result.Next(txn) {
			ionBinary := result.GetCurrentData()
			// unmarshal enriched version
			err := ion.Unmarshal(ionBinary, resp)
			if err != nil {
				return nil, fmt.Errorf("failed to unmarshal tx: %s due to: %w", docID, err)
			}
		}

		return resp, nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get transactions: %w", err)
	}
	return transaction.(*Transaction), nil
}
