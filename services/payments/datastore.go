package payments

import (
	"context"
	"errors"
	"fmt"

	smithy "github.com/aws/smithy-go"
	"github.com/brave-intl/bat-go/libs/logging"
	appaws "github.com/brave-intl/bat-go/libs/nitro/aws"
	"github.com/rs/zerolog"

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

type Datastore interface {
	InsertPaymentState(ctx context.Context, state *paymentLib.PaymentState) (string, error)
	InsertChainAddress(ctx context.Context, address ChainAddress) (string, error)
	InsertVault(ctx context.Context, vault Vault) error
	GetPaymentStateHistory(ctx context.Context, documentID string) (*paymentLib.PaymentStateHistory, error)
	GetChainAddress(ctx context.Context, address string) (*ChainAddress, error)
	GetVault(ctx context.Context, pubkey string) (*Vault, error)
	GetVaultWithPublicKey(ctx context.Context, pubkey string) (*Vault, error)
	UpdatePaymentState(ctx context.Context, documentID string, state *paymentLib.PaymentState) error
	UpdateChainAddress(ctx context.Context, address ChainAddress) error
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
			}
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
		},
	)
	if err != nil {
		return "", fmt.Errorf("failed to insert transaction: %w", err)
	}
	return insertedDocumentID.(string), nil
}

func (q *QLDBDatastore) InsertChainAddress(ctx context.Context, address ChainAddress) (string, error) {
	insertedDocumentID, err := q.Execute(
		context.Background(),
		func(txn qldbdriver.Transaction) (interface{}, error) {
			// For the transaction, check if this transaction has already existed. If not, perform
			// the insertion.
			existingkey, err := txn.Execute(
				"SELECT d_id as documentID FROM chainaddresses BY d_id where publicKey = ?",
				address.PublicKey,
			)
			if err != nil {
				return nil, fmt.Errorf(
					"failed to insert key: %s due to: %w",
					address.PublicKey,
					err,
				)
			}

			// Check if there are any results. If there are, we should skip insert.
			if existingkey.Next(txn) {
				documentIDResult := new(qldbDocumentIDResult)
				err = ion.Unmarshal(existingkey.GetCurrentData(), &documentIDResult)
				if err != nil {
					return nil, err
				}
				return documentIDResult.DocumentID, nil
			}
			documentIDResultBinary, err := txn.Execute(
				"INSERT INTO chainaddresses ?",
				address,
			)
			if err != nil {
				return nil, fmt.Errorf(
					"failed to insert key: %s due to: %w",
					address.PublicKey,
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
				"failed to insert key: %s due to: %w",
				address.PublicKey,
				err,
			)
		},
	)
	if err != nil {
		return "", fmt.Errorf("failed to insert key: %s due to: %w", address.PublicKey, err)
	}
	return insertedDocumentID.(string), nil
}

// InsertVault checks if a vault already exists. If it does, it returns with no error and no changes.
// If the vault does not exist, it is inserted into QLDB.
func (q *QLDBDatastore) InsertVault(ctx context.Context, vault Vault) error {
	_, err := q.Execute(
		context.Background(),
		func(txn qldbdriver.Transaction) (interface{}, error) {
			// Check if this vault's idempotency key already exists. If not, perform the
			// insertion. We can't use the public key in this case because the key will
			// be recently genereated and different from any existing keys at this point.
			existingkey, err := txn.Execute(
				"SELECT d_id as documentID FROM vaults BY d_id where publicKey = ?",
				vault.PublicKey,
			)
			if err != nil {
				return nil, fmt.Errorf(
					"failed to insert key: %s due to: %w",
					vault.PublicKey,
					err,
				)
			}

			// Check if there are any results. If there are, we should skip insert.
			if existingkey.Next(txn) {
				documentIDResult := new(qldbDocumentIDResult)
				err = ion.Unmarshal(existingkey.GetCurrentData(), &documentIDResult)
				if err != nil {
					return nil, err
				}
				return documentIDResult.DocumentID, nil
			}
			documentIDResultBinary, err := txn.Execute(
				"INSERT INTO vaults ?",
				vault,
			)
			if err != nil {
				return nil, fmt.Errorf(
					"failed to insert key: %s due to: %w",
					vault.PublicKey,
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
			return nil, fmt.Errorf("failed to insert key: %s due to: %w", vault.PublicKey, err)
		},
	)
	if err != nil {
		return fmt.Errorf("failed to insert key: %s due to: %w", vault.PublicKey, err)
	}
	return nil
}

func (q *QLDBDatastore) GetPaymentStateHistory(ctx context.Context, documentID string) (*paymentLib.PaymentStateHistory, error) {
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

func (q *QLDBDatastore) GetChainAddress(ctx context.Context, address string) (*ChainAddress, error) {
	chainAddress, err := q.Execute(context.Background(), func(txn qldbdriver.Transaction) (interface{}, error) {
		result, err := txn.Execute(
			"SELECT * FROM chainaddresses WHERE publicKey = ?",
			address,
		)
		if err != nil {
			return nil, fmt.Errorf("QLDB transaction failed: %w", err)
		}
		var chainAddress ChainAddress
		for result.Next(txn) {
			err := ion.Unmarshal(result.GetCurrentData(), &chainAddress)
			if err != nil {
				return nil, fmt.Errorf("ion unmarshal failed: %w", err)
			}
		}

		return &chainAddress, nil
	})
	if err != nil {
		return nil, err
	}
	return chainAddress.(*ChainAddress), err
}

// GetVault fetches a vault from QLDB using both the public key and the idempotency key
func (q *QLDBDatastore) GetVault(
	ctx context.Context,
	publicKey string,
) (*Vault, error) {
	vault, err := q.Execute(context.Background(), func(txn qldbdriver.Transaction) (interface{}, error) {
		result, err := txn.Execute(
			"SELECT * FROM vaults WHERE publicKey = ?",
			publicKey,
		)
		if err != nil {
			return nil, fmt.Errorf("QLDB transaction failed: %w", err)
		}
		var vault Vault
		for result.Next(txn) {
			err := ion.Unmarshal(result.GetCurrentData(), &vault)
			if err != nil {
				return nil, fmt.Errorf("ion unmarshal failed: %w", err)
			}
		}

		return &vault, nil
	})
	if err != nil {
		return nil, err
	}
	return vault.(*Vault), err
}

func (q *QLDBDatastore) GetVaultWithPublicKey(ctx context.Context, publicKey string) (*Vault, error) {
	vault, err := q.Execute(context.Background(), func(txn qldbdriver.Transaction) (interface{}, error) {
		result, err := txn.Execute(
			"SELECT * FROM vaults WHERE publicKey = ?",
			publicKey,
		)
		if err != nil {
			return nil, fmt.Errorf("QLDB transaction failed: %w", err)
		}
		var vault Vault
		for result.Next(txn) {
			err := ion.Unmarshal(result.GetCurrentData(), &vault)
			if err != nil {
				return nil, fmt.Errorf("ion unmarshal failed: %w", err)
			}
		}

		return &vault, nil
	})
	if err != nil {
		return nil, err
	}
	return vault.(*Vault), err
}

// UpdateChainAddress adds approvals to an existing chain address
func (q *QLDBDatastore) UpdateChainAddress(ctx context.Context, address ChainAddress) error {
	_, err := q.Execute(
		ctx,
		func(txn qldbdriver.Transaction) (interface{}, error) {
			_, err := txn.Execute(
				`UPDATE chainaddresses
					SET
						publicKey = ?,
						approvals = ?
					WHERE
						publicKey = ?
				`,
				address.PublicKey,
				address.Approvals,
				address.PublicKey,
			)
			return nil, err
		},
	)
	if err != nil {
		return fmt.Errorf("QLDB write execution failed: %w", err)
	}
	return nil
}

func configureDatastore(ctx context.Context) (Datastore, error) {
	driver, err := newQLDBDatastore(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create new qldb datastore: %w", err)
	}
	err = driver.setupLedger(ctx)
	if err != nil {
		return nil, err
	}
	return driver, nil
}

func ensureQLDBTable(
	txn qldbdriver.Transaction,
	tableName string,
	logger *zerolog.Logger,
) (bool, error) {
	// Check for table existence. In QLDB DROP TABLE does not delete the table
	// but merely marks it as inactive and it is still reachable by its id.
	//
	// NOTE: previously the code relied on CREATE TABLE reporting an error when
	// table did not exist and then checked for a particular error code to
	// distinguish between non-existing table and other errors. But error codes
	// are not stable with QLDB client, so explicit query against the
	// information_schema should be more robust.
	r, err := txn.Execute("SELECT tableId FROM information_schema.user_tables WHERE name = ? AND status = 'ACTIVE'", tableName)
	if err != nil {
		return false, fmt.Errorf(
			"failed to query user_tables for %s: %w", tableName, err)
	}
	if r.Next(txn) {
		// We have an entry for the table so it exists.
		return false, nil
	}

	logger.Warn().Err(err).Msgf("Failed to locate %s table, creating one", tableName)
	_, err = txn.Execute(fmt.Sprintf("CREATE TABLE %s", tableName))
	if err != nil {
		return false, fmt.Errorf(
			"failed to create %s table due to: %w", tableName, err)
	}
	return true, nil
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
				return nil, fmt.Errorf("failed to create authorizations table due to: %w", err)
			}
		}
		ok = false
		_, err = txn.Execute("CREATE TABLE chainaddresses")
		if err != nil {
			logger.Warn().Err(err).Msg("error creating chainaddresses table")
			if errors.As(err, &ae) {
				logger.Warn().Err(err).Str("code", ae.ErrorCode()).Msg("api error creating chainaddresses table")
				if ae.ErrorCode() == tableAlreadyCreatedCode {
					// table has already been created
					ok = true
				}
			}
			if !ok {
				return nil, fmt.Errorf("failed to create chainaddresses table due to: %w", err)
			}
		} else {
			_, err = txn.Execute("CREATE INDEX ON chainaddresses (publicKey)")
			if err != nil {
				return nil, err
			}
		}
		ok = false
		_, err = txn.Execute("CREATE TABLE vaults")
		if err != nil {
			logger.Warn().Err(err).Msg("error creating vaults table")
			if errors.As(err, &ae) {
				logger.Warn().Err(err).Str("code", ae.ErrorCode()).Msg("api error creating vaults table")
				if ae.ErrorCode() == tableAlreadyCreatedCode {
					// table has already been created
					ok = true
				}
			}
			if !ok {
				return nil, fmt.Errorf("failed to create vaults table due to: %w", err)
			}
		} else {
			_, err = txn.Execute("CREATE INDEX ON vaults (publicKey)")
			if err != nil {
				return nil, err
			}
			_, err = txn.Execute("CREATE INDEX ON vaults (idempotencyKey)")
			if err != nil {
				return nil, err
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
