package payments

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/aws/smithy-go"
	"github.com/brave-intl/bat-go/libs/logging"
	appaws "github.com/brave-intl/bat-go/libs/nitro/aws"

	"github.com/amazon-ion/ion-go/ion"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmsTypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/aws/aws-sdk-go-v2/service/qldbsession"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/awslabs/amazon-qldb-driver-go/v3/qldbdriver"
	appctx "github.com/brave-intl/bat-go/libs/context"
	. "github.com/brave-intl/bat-go/libs/payments"
	"github.com/google/uuid"
)

type qldbDocumentIDResult struct {
	documentID string `ion:"documentId"`
}

// ErrNotConfiguredYet - service not fully configured.
var ErrNotConfiguredYet = errors.New("not yet configured")

const (
	// StatePrepared - transaction prepared state
	StatePrepared = "prepared"
	// StateSubmitted - transaction prepared state
	StateSubmitted          = "submitted"
	tableAlreadyCreatedCode = "412"
)

// signPaymentState - perform KMS signing of the transaction, return publicKey and
// signature in hex string.
func signPaymentState(
	ctx context.Context,
	kmsClient wrappedKMSClient,
	keyID string,
	state PaymentState,
) (string, string, error) {
	pubkeyOutput, err := kmsClient.GetPublicKey(ctx, &kms.GetPublicKeyInput{
		KeyId: &keyID,
	})

	if err != nil {
		return "", "", fmt.Errorf("Failed to get public key: %w", err)
	}

	signingOutput, err := kmsClient.Sign(ctx, &kms.SignInput{
		KeyId:            &keyID,
		Message:          state.UnsafePaymentState,
		SigningAlgorithm: kmsTypes.SigningAlgorithmSpecEcdsaSha256,
	})

	if err != nil {
		return "", "", fmt.Errorf("Failed to sign transaction: %w", err)
	}

	return hex.EncodeToString(pubkeyOutput.PublicKey), hex.EncodeToString(signingOutput.Signature), nil
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

// insertPayment - perform a qldb insertion on a given transaction after doing all validation.
// Returns the documentID for the record that was inserted.
func (s Service) insertPayment(
	ctx context.Context,
	details PaymentDetails,
) (string, error) {
	unsignedPaymentState, err := PaymentStateFromDetails(details, s.idempotencyNamespace)
	if err != nil {
		return "", err
	}
	_, signature, err := signPaymentState(
		ctx,
		s.kmsSigningClient,
		s.kmsDecryptKeyArn,
		*unsignedPaymentState,
	)
	if err != nil {
		return "", err
	}

	paymentState := PaymentState{
		UnsafePaymentState: unsignedPaymentState.UnsafePaymentState,
		ID: unsignedPaymentState.ID,
		Signature: []byte(signature),
	}

	insertedDocumentID, err := s.datastore.Execute(
		context.Background(),
		func(txn qldbdriver.Transaction) (interface{}, error) {
			// For the transaction, check if this transaction has already existed. If not, perform
			// the insertion.
			existingPaymentState, err := txn.Execute(
				"SELECT * FROM transactions WHERE idempotencyKey = ?",
				paymentState.ID,
			)
			if err != nil {
				return nil, fmt.Errorf("failed to insert tx: %s due to: %w", paymentState.ID, err)
			}

			// Check if there are any results. There should not be. If there are, we should not
			// insert.
			if !existingPaymentState.Next(txn) {
				documentIDResultBinary, err := txn.Execute("INSERT INTO transactions ?", paymentState)
				if err != nil {
					return nil, fmt.Errorf(
						"failed to insert tx: %s due to: %w",
						paymentState.ID,
						err,
					)
				}
				ionBinary := documentIDResultBinary.GetCurrentData()
				documentIDResult := new(qldbDocumentIDResult)
				err = ion.Unmarshal(ionBinary, documentIDResult)
				if err != nil {
					return nil, err
				}
				return documentIDResult.documentID, nil
			}

			return nil, fmt.Errorf(
				"failed to insert transaction because id already exists: %s",
				paymentState.ID,
			)
		},
	)
	if err != nil {
		return "", fmt.Errorf("failed to insert transaction: %w", err)
	}
	return insertedDocumentID.(string), nil
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
	fetchedTxn, idempotencyKey, err := s.GetTransactionFromDocumentID(ctx, transaction.DocumentID)
	if err != nil {
		return fmt.Errorf(
			"failed to get transaction with idempotencyKey %s by document ID %s: %w",
			idempotencyKey,
			transaction.DocumentID,
			err,
		)
	}
	auth := PaymentAuthorization{
		KeyID:      keyID,
		DocumentID: transaction.DocumentID,
	}
	keyHasNotYetSigned := true
	for _, authorization := range fetchedTxn.Authorizations {
		if authorization.KeyID == auth.KeyID {
			keyHasNotYetSigned = false
		}
	}
	if !keyHasNotYetSigned {
		return fmt.Errorf("key %s has already signed document %s", auth.KeyID, fetchedTxn.DocumentID)
	}
	fetchedTxn.Authorizations = append(fetchedTxn.Authorizations, auth)
	writtenTxn, err := writeTransaction(
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
	stateMachine, err := StateMachineFromTransaction(s, writtenTxn)
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
) (*AuthenticatedPaymentState, uuid.UUID, error) {
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
		return nil, uuid.New(), fmt.Errorf("failed to get transactions: %w", err)
	}
	paymentState := paymentStateInterface.(*PaymentState)
	authenticatedState, err := paymentState.ToAuthenticatedPaymentState()
	if err != nil {
		return nil, uuid.New(), err
	}
	authenticatedState.DocumentID = documentID
	return authenticatedState, paymentState.ID, nil
}

// getTransactionByIdempotencyKey returns the latest version of a record from QLDB if it exists, after doing all requisite validation.
func getTransactionByIdempotencyKey(
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

// getLatestPaymentHistoryEntry returns the latest version of an entry for a given ID after doing
// all requisite validation.
func getLatestPaymentHistoryEntry(
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

// writeTransaction persists an object in a transaction after verifying that its change
// represents a valid state transition.
func writeTransaction(
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
			_, err := getLatestPaymentHistoryEntry(ctx, txn, sdkClient, kmsSigningClient, transaction.DocumentID)
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
			_, signature, err := signPaymentState(
				ctx,
				kmsSigningClient,
				kmsSigningKeyID,
				// @TODO FIX THIS
				AuthenticatedPaymentState{},
			)
			if err != nil {
				return nil, fmt.Errorf("failed to sign transaction: %w", err)
			}

			return txn.Execute(
				"UPDATE transactions BY d_id SET data = ?, signature = ? WHERE d_id = ?",
				result,
				signature,
				transaction.DocumentID,
			)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("QLDB write execution failed: %w", err)
	}
	return transaction, nil
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
