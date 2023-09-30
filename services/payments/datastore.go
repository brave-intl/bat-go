package payments

import (
	"context"
	"encoding/json"
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
	. "github.com/brave-intl/bat-go/libs/payments"
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
	InsertPaymentState(ctx context.Context, state *PaymentState) (string, error)
	GetPaymentStateHistory(ctx context.Context, documentID string) (*PaymentStateHistory, error)
	UpdatePaymentState(ctx context.Context, documentID string, state *PaymentState) error
}

func (q *QLDBDatastore) InsertPaymentState(ctx context.Context, state *PaymentState) (string, error) {
	insertedDocumentID, err := q.Execute(
		context.Background(),
		func(txn qldbdriver.Transaction) (interface{}, error) {
			// For the transaction, check if this transaction has already existed. If not, perform
			// the insertion.
			existingPaymentState, err := txn.Execute(
				"SELECT * FROM transactions WHERE idempotencyKey = ?",
				state.ID,
			)
			if err != nil {
				return nil, fmt.Errorf(
					"failed to insert tx: %s due to: %w",
					state.ID,
					err,
				)
			}

			// Check if there are any results. There should not be. If there are, we should not
			// insert.
			if !existingPaymentState.Next(txn) {
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
				if err != nil {
					return nil, fmt.Errorf(
						"failed to insert tx: %s due to: %w",
						state.ID,
						err,
					)
				}
			}

			return nil, fmt.Errorf(
				"failed to insert transaction because id already exists: %s",
				state.ID,
			)
		},
	)
	if err != nil {
		return "", fmt.Errorf("failed to insert transaction: %w", err)
	}
	return insertedDocumentID.(string), nil
}

func (q *QLDBDatastore) GetPaymentStateHistory(ctx context.Context, documentID string) (*PaymentStateHistory, error) {
	stateHistory, err := q.Execute(context.Background(), func(txn qldbdriver.Transaction) (interface{}, error) {
		result, err := txn.Execute(
			"SELECT * FROM history(transactions) AS h WHERE h.metadata.id = ?",
			documentID,
		)
		if err != nil {
			return nil, fmt.Errorf("QLDB transaction failed: %w", err)
		}
		var stateHistory []PaymentState
		var latestHistoryItem QLDBPaymentTransitionHistoryEntry
		for result.Next(txn) {
			err := ion.Unmarshal(result.GetCurrentData(), &latestHistoryItem)
			if err != nil {
				return nil, fmt.Errorf("ion unmarshal failed: %w", err)
			}
			stateHistory = append(stateHistory, latestHistoryItem.Data)
		}

		if len(stateHistory) < 1 {
			return nil, &QLDBTransitionHistoryNotFoundError{}
		}

		merkleValid, err := revisionValidInTree(ctx, q.sdkClient, &latestHistoryItem)
		if err != nil {
			return nil, fmt.Errorf("failed to verify Merkle tree: %w", err)
		}
		if !merkleValid {
			return nil, fmt.Errorf("invalid Merkle tree for record: %#v", latestHistoryItem)
		}

		tmp := PaymentStateHistory(stateHistory)
		return &tmp, nil
	})

	return stateHistory.(*PaymentStateHistory), err
}

func (q *QLDBDatastore) UpdatePaymentState(ctx context.Context, documentID string, state *PaymentState) error {
	_, err := q.Execute(
		ctx,
		func(txn qldbdriver.Transaction) (interface{}, error) {
			_, err := txn.Execute(
				"UPDATE transactions BY d_id SET data = ?, signature = ?, publicKey = ? WHERE d_id = ?",
				state.UnsafePaymentState,
				state.Signature,
				state.PublicKey,
				documentID,
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
	return s.setupLedger(ctx)
}

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

	return &QLDBDatastore{QLDBDriver: driver}, nil
}

// insertPayment - perform a qldb insertion on a given transaction after doing all validation.
// Returns the documentID for the record that was inserted.
func (s Service) insertPayment(
	ctx context.Context,
	details PaymentDetails,
) (string, error) {
	authenticatedState := details.ToAuthenticatedPaymentState()

	// Ensure that prepare succeeds ( i.e. we are not using a failing dry-run state machine )
	stateMachine, err := StateMachineFromTransaction(&s, authenticatedState)
	if err != nil {
		return "", fmt.Errorf("failed to create stateMachine: %w", err)
	}
	_, err = stateMachine.Prepare(ctx)
	if err != nil {
		return "", err
	}

	paymentStateForSigning, err := authenticatedState.ToPaymentState()
	if err != nil {
		return "", err
	}
	pubkey, signature, err := signPaymentState(
		ctx,
		s.kmsSigningClient,
		s.kmsSigningKeyID,
		*paymentStateForSigning,
	)
	if err != nil {
		return "", err
	}
	paymentStateForSigning.Signature = signature
	paymentStateForSigning.PublicKey = pubkey

	return s.datastore.InsertPaymentState(ctx, paymentStateForSigning)
}

// GetTransactionFromDocumentID - get the transaction data from the document ID in qldb.
func (s *Service) GetTransactionFromDocumentID(
	ctx context.Context,
	documentID string,
) (*AuthenticatedPaymentState, error) {
	history, err := s.datastore.GetPaymentStateHistory(ctx, documentID)
	if err != nil {
		return nil, err
	}
	authenticatedState, err := history.GetAuthenticatedPaymentState(s.verifier, documentID)
	if err != nil {
		return nil, err
	}

	return authenticatedState, nil
}

// writeTransaction persists an object in a transaction after verifying that its change
// represents a valid state transition.
func writeTransaction(
	ctx context.Context,
	datastore compatDatastore,
	sdkClient wrappedQldbSDKClient,
	kmsSigningClient wrappedKMSClient,
	kmsSigningKeyID string,
	authenticatedState *AuthenticatedPaymentState,
) (*AuthenticatedPaymentState, error) {
	marshaledState, err := json.Marshal(authenticatedState)
	if err != nil {
		return nil, err
	}

	paymentState := PaymentState{
		UnsafePaymentState: marshaledState,
	}

	// ignore public key
	pubkey, signature, err := signPaymentState(
		ctx,
		kmsSigningClient,
		kmsSigningKeyID,
		paymentState,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}
	paymentState.Signature = []byte(signature)
	paymentState.PublicKey = pubkey

	return authenticatedState, datastore.UpdatePaymentState(ctx, authenticatedState.DocumentID, &paymentState)
}
