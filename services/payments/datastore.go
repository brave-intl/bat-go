package payments

import (
	"context"
	"errors"
	"fmt"

	smithy "github.com/aws/smithy-go"
	"github.com/brave-intl/bat-go/libs/logging"
	appaws "github.com/brave-intl/bat-go/libs/nitro/aws"

	"github.com/amazon-ion/ion-go/ion"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/qldb"
	"github.com/aws/aws-sdk-go-v2/service/qldbsession"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/awslabs/amazon-qldb-driver-go/v3/qldbdriver"
	appctx "github.com/brave-intl/bat-go/libs/context"
	paymentLib "github.com/brave-intl/bat-go/libs/payments"
)

type qldbDocumentIDResult struct {
	DocumentID string `ion:"documentId"`
}

type QLDBDatastore struct {
	*qldbdriver.QLDBDriver
	sdkClient *qldb.Client
}

// ErrNotConfiguredYet - service not fully configured.
var ErrNotConfiguredYet = errors.New("not yet configured")

const (
	tableAlreadyCreatedCode = "412"
)

type Datastore interface {
	InsertPaymentState(ctx context.Context, state *paymentLib.PaymentState) (string, error)
	GetPaymentStateHistory(ctx context.Context, documentID string) (*paymentLib.PaymentStateHistory, error)
	UpdatePaymentState(ctx context.Context, documentID string, state *paymentLib.PaymentState) error
}

func (q *QLDBDatastore) InsertPaymentState(ctx context.Context, state *paymentLib.PaymentState) (string, error) {
	insertedDocumentID, err := q.Execute(
		context.Background(),
		func(txn qldbdriver.Transaction) (interface{}, error) {
			// For the transaction, check if this transaction has already existed. If not, perform
			// the insertion.
			existingPaymentState, err := txn.Execute(
				"SELECT d_id as documentID FROM transactions BY d_id where idempotencyKey = ?",
				state.ID,
			)
			if err != nil {
				return nil, fmt.Errorf(
					"failed to insert tx: %s due to: %w",
					state.ID,
					err,
				)
			}

			// Check if there are any results. If there are, we should skip insert.
			if existingPaymentState.Next(txn) {
				documentIDResult := new(qldbDocumentIDResult)
				err = ion.Unmarshal(existingPaymentState.GetCurrentData(), &documentIDResult)
				if err != nil {
					return nil, err
				}
				return documentIDResult.DocumentID, nil
			} else {
				documentIDResultBinary, err := txn.Execute(
					"INSERT INTO transactions ?",
					state,
				)
				if err != nil {
					return nil, fmt.Errorf(
						"failed to insert tx: %s due to: %w",
						state.ID,
						err,
					)
				}

				if documentIDResultBinary.Next(txn) {
					documentIDResult := new(qldbDocumentIDResult)
					err = ion.Unmarshal(documentIDResultBinary.GetCurrentData(), &documentIDResult)
					if err != nil {
						return nil, err
					}
					return documentIDResult.DocumentID, nil
				}

				err = documentIDResultBinary.Err()
				return nil, fmt.Errorf(
					"failed to insert tx: %s due to: %w",
					state.ID,
					err,
				)
			}
		},
	)
	if err != nil {
		return "", fmt.Errorf("failed to insert transaction: %w", err)
	}
	return insertedDocumentID.(string), nil
}

func (q *QLDBDatastore) GetPaymentStateHistory(ctx context.Context, documentID string) (*paymentLib.PaymentStateHistory, error) {
	logger := logging.Logger(ctx, "payments.setupLedger")

	stateHistory, err := q.Execute(context.Background(), func(txn qldbdriver.Transaction) (interface{}, error) {
		result, err := txn.Execute(
			"SELECT * FROM history(transactions) AS h WHERE h.metadata.id = ?",
			documentID,
		)
		if err != nil {
			return nil, fmt.Errorf("QLDB transaction failed: %w", err)
		}
		var stateHistory []paymentLib.PaymentState
		var latestHistoryItem QLDBPaymentTransitionHistoryEntry
		for result.Next(txn) {
			err := ion.Unmarshal(result.GetCurrentData(), &latestHistoryItem)
			if err != nil {
				return nil, fmt.Errorf("ion unmarshal failed: %w", err)
			}
			latestHistoryItem.Data.UpdatedAt = latestHistoryItem.Metadata.TxTime
			stateHistory = append(stateHistory, latestHistoryItem.Data)
		}

		if len(stateHistory) < 1 {
			return nil, &QLDBTransitionHistoryNotFoundError{}
		}

		merkleValid, err := revisionValidInTree(ctx, q.sdkClient, &latestHistoryItem)
		if err != nil {
			//return nil, fmt.Errorf("failed to verify Merkle tree: %w", err)
			logger.Warn().Err(err).Msg("failed to verify Merkle tree")
		}
		if !merkleValid {
			//return nil, fmt.Errorf("invalid Merkle tree for record: %#v", latestHistoryItem)
			logger.Warn().Msg("invalid Merkle tree for record")
		}

		tmp := paymentLib.PaymentStateHistory(stateHistory)
		return &tmp, nil
	})

	if err != nil {
		return nil, err
	}
	return stateHistory.(*paymentLib.PaymentStateHistory), err
}

func (q *QLDBDatastore) UpdatePaymentState(ctx context.Context, documentID string, state *paymentLib.PaymentState) error {
	_, err := q.Execute(
		ctx,
		func(txn qldbdriver.Transaction) (interface{}, error) {
			_, err := txn.Execute(
				`UPDATE transactions BY d_id
						SET
							data = ?,
							signature = ?,
							publicKey = ?
						WHERE
							d_id = ?
							AND data != ?
				`,
				state.UnsafePaymentState,
				state.Signature,
				state.PublicKey,
				documentID,
				state.UnsafePaymentState,
			)
			return nil, err
		},
	)
	if err != nil {
		return fmt.Errorf("QLDB write execution failed: %w", err)
	}
	return nil
}

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
	return driver.setupLedger(ctx)
}

func (q *QLDBDatastore) setupLedger(ctx context.Context) error {
	logger := logging.Logger(ctx, "payments.setupLedger")
	// create the tables needed in the ledger
	_, err := q.Execute(context.Background(), func(txn qldbdriver.Transaction) (interface{}, error) {
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
		} else {
			_, err = txn.Execute("CREATE INDEX ON transactions (idempotencyKey)")
			if err != nil {
				return nil, err
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

// newQLDBDatastore - create a new qldbDatastore.
func newQLDBDatastore(ctx context.Context) (*QLDBDatastore, error) {
	logger := logging.Logger(ctx, "payments.newQLDBDatastore")

	if !isQLDBReady(ctx) {
		return nil, ErrNotConfiguredYet
	}

	egressProxyAddr, ok := ctx.Value(appctx.EgressProxyAddrCTXKey).(string)
	if !ok {
		return nil, fmt.Errorf("failed to get egress proxy for qldb")
	}

	// the aws region
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

	return &QLDBDatastore{QLDBDriver: driver, sdkClient: qldb.NewFromConfig(awsCfg)}, nil
}
