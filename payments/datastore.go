package payments

import (
	"context"
	"fmt"

	"github.com/amzn/ion-go/ion"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/qldbsession"
	"github.com/awslabs/amazon-qldb-driver-go/v2/qldbdriver"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Transaction - the main type explaining a transaction, type used for qldb via ion
type Transaction struct {
	IdempotencyKey *uuid.UUID      `json:"idempotencyKey,omitempty" ion:"idempotencyKey" valid:"uuid"`
	Amount         decimal.Decimal `json:"amount,omitempty" ion:"amount" valid:"numeric"`
	To             *uuid.UUID      `json:"to,omitempty" ion:"to" valid:"required"`
	From           *uuid.UUID      `json:"from,omitempty" ion:"from" valid:"required"`
	Custodian      string          `json:"custodian,omitempty" ion:"custodian" valid:"in(uphold|gemini|bitflyer)"`
	State          string          `json:"-" ion:"state" valid:"in(prepared|submitted)"`
	DocumentID     string          `json:"documentId,omitempty" ion:"id"`
}

// newQLDBDatastore - create a new qldbDatastore
func newQLDBDatastore(ctx context.Context) (*qldbdriver.QLDBDriver, error) {
	// create our aws session
	awsSession := session.Must(session.NewSession())
	// create our qldb driver
	qldbSession := qldbsession.New(awsSession)
	// create our qldb driver
	driver, err := qldbdriver.New(
		"payments-service",
		qldbSession,
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

// InsertTransactions - perform a qldb insertion on the transactions
func (s Service) InsertTransactions(ctx context.Context, transactions ...*Transaction) ([]Transaction, error) {
	enrichedTransactions, err := s.datastore.Execute(context.Background(), func(txn qldbdriver.Transaction) (interface{}, error) {
		// for all of the transactions load up a check to see if this transaction has already existed
		// or not, then perform the insertion of the records.
		resp := []Transaction{}

		for _, transaction := range transactions {
			// Check if a document with this idempotencyKey exists
			result, err := txn.Execute("SELECT * FROM transaction WHERE idempotencyKey = ?", transaction.IdempotencyKey)
			if err != nil {
				return nil, fmt.Errorf("failed to insert tx: %s due to: %w", transaction.IdempotencyKey, err)
			}
			// Check if there are any results
			if !result.Next(txn) {
				// set transaction state to prepared
				transaction.State = StatePrepared
				// insert the transaction
				_, err = txn.Execute("INSERT INTO transaction ?", transaction)
				if err != nil {
					return nil, fmt.Errorf("failed to insert tx: %s due to: %w", transaction.IdempotencyKey, err)
				}
			}
			// get the document id for the inserted transaction
			result, err = txn.Execute("SELECT data.*, metadata.id FROM _ql_committed_transaction as t WHERE t.data.idempotencyKey = ?", transaction.IdempotencyKey)
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
				// add to response
				resp = append(resp, *enriched)
			}

		}
		return resp, nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to insert transactions: %w", err)
	}
	return enrichedTransactions.([]Transaction), nil
}

// UpdateTransactionsState - Change transaction state
func (s Service) UpdateTransactionsState(ctx context.Context, state string, transactions ...Transaction) error {
	_, err := s.datastore.Execute(context.Background(), func(txn qldbdriver.Transaction) (interface{}, error) {
		// for all of the transactions load up a check to see if this transaction has already existed
		// or not, then perform the insertion of the records.
		for _, transaction := range transactions {
			// Check if a document with this idempotencyKey exists
			result, err := txn.Execute("SELECT * FROM transaction WHERE idempotencyKey = ?", transaction.IdempotencyKey)
			if err != nil {
				return nil, fmt.Errorf("failed to update tx: %s due to: %w", transaction.IdempotencyKey, err)
			}
			// Check if there are any results
			if result.Next(txn) {
				// update the transaction state
				_, err = txn.Execute("UPDATE transaction SET state = ? WHERE idempotencyKey = ?", state, transaction.IdempotencyKey)
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

// AuthorizeTransactions - Add an Authorization for the Transaction
func (s Service) AuthorizeTransactions(ctx context.Context, keyID string, transactions ...Transaction) error {
	_, err := s.datastore.Execute(context.Background(), func(txn qldbdriver.Transaction) (interface{}, error) {
		// for all of the transactions load up a check to see if this transaction has already existed
		// or not, then perform the insertion of the records.
		for _, transaction := range transactions {
			auth := map[string]string{
				"keyId":      keyID,
				"documentId": transaction.DocumentID,
			}
			_, err := txn.Execute("INSERT INTO authorizations ?", auth)
			if err != nil {
				return nil, fmt.Errorf("failed to insert tx authorization: %+v due to: %w", auth, err)
			}
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

		result, err := txn.Execute("SELECT data.*,metadata.id FROM _ql_committed_transaction WHERE metadata.id = ?", docID)
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
