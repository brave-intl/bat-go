package payments

import (
	"context"
	"fmt"

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
	To             string          `json:"to,omitempty" ion:"to" valid:"required"`
	From           string          `json:"from,omitempty" ion:"from" valid:"required"`
	Custodian      string          `json:"custodian" ion:"custodian" valid:"in(uphold|gemini|bitflyer)"`
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

// InsertTransactions - perform a qldb insertion on the transactions
func (s Service) InsertTransactions(ctx context.Context, transactions ...*Transaction) error {
	_, err := s.datastore.Execute(context.Background(), func(txn qldbdriver.Transaction) (interface{}, error) {
		// for all of the transactions load up a check to see if this transaction has already existed
		// or not, then perform the insertion of the records.
		for _, transaction := range transactions {
			// Check if a document with this idempotencyKey exists
			result, err := txn.Execute("SELECT * FROM transaction WHERE idempotencyKey = ?", transaction.IdempotencyKey)
			if err != nil {
				return nil, fmt.Errorf("failed to insert tx: %s due to: %w", transaction.IdempotencyKey, err)
			}
			// Check if there are any results
			if result.Next(txn) {
				// Document already exists, no need to insert
			} else {
				_, err = txn.Execute("INSERT INTO transaction ?", transaction)
				if err != nil {
					return nil, fmt.Errorf("failed to insert tx: %s due to: %w", transaction.IdempotencyKey, err)
				}
			}
		}
		return nil, nil
	})
	if err != nil {
		return fmt.Errorf("failed to insert transactions: %w", err)
	}
	return nil
}
