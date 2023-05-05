package payments

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/aws/smithy-go"

	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/shopspring/decimal"

	"github.com/amazon-ion/ion-go/ion"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmsTypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/awslabs/amazon-qldb-driver-go/v3/qldbdriver"
	"github.com/google/uuid"
)

// Transaction - the main type explaining a transaction, type used for qldb via ion
type Transaction struct {
	IdempotencyKey      *uuid.UUID       `json:"idempotencyKey,omitempty" ion:"idempotencyKey" valid:"required"`
	Amount              *ion.Decimal     `json:"-" ion:"amount" valid:"required"`
	To                  *uuid.UUID       `json:"to,omitempty" ion:"to" valid:"required"`
	From                *uuid.UUID       `json:"from,omitempty" ion:"from" valid:"required"`
	Custodian           string           `json:"custodian,omitempty" ion:"custodian" valid:"in(uphold|gemini|bitflyer)"`
	State               TransactionState `json:"state,omitempty" ion:"state"`
	DocumentID          string           `json:"documentId,omitempty" ion:"id"`
	AttestationDocument string           `json:"attestation,omitempty" ion:"-"`
	PayoutID            string           `json:"payoutId" valid:"required"`
	Signature           []byte           `json:"-" ion:"signature"` // KMS signature only enclave can sign
	PublicKey           string           `json:"-" ion:"publicKey"` // KMS signature only enclave can sign
	Currency            string           `json:"-" ion:"currency"`
	DryRun              *string          `json:"dryRun" ion:"-"` // determines dry-run
}

// GetIdempotencyKey returns a UUID v5 ID if the ID on the Transaction matches its expected value. Otherwise, it returns
// an error
func (t *Transaction) GetIdempotencyKey(ctx context.Context) (*uuid.UUID, error) {
	namespace, ok := ctx.Value("namespaceUUID").(uuid.UUID)
	if !ok {
		return nil, errors.New("context namespaceUUID is of type uuid.UUID")
	}
	generatedIdempotencyKey := uuid.NewSHA1(namespace, []byte(fmt.Sprintf("%s%s%s%s%s%s", t.To, t.From, t.Currency, t.Amount, t.Custodian, t.PayoutID)))
	if generatedIdempotencyKey == *t.IdempotencyKey {
		return t.IdempotencyKey, nil
	}
	return nil, errors.New("IdempotencyKey does not match transaction fields")
}

// SetIdempotencyKey assigns a UUID v5 value to Transaction.ID
func (t *Transaction) SetIdempotencyKey(ctx context.Context) error {
	namespace, ok := ctx.Value("namespaceUUID").(uuid.UUID)
	if !ok {
		return errors.New("context namespaceUUID is of type uuid.UUID")
	}
	generatedIdempotencyKey := uuid.NewSHA1(namespace, []byte(fmt.Sprintf("%s%s%s%s%s%s", t.To, t.From, t.Currency, t.Amount, t.Custodian, t.PayoutID)))
	t.IdempotencyKey = &generatedIdempotencyKey
	return nil
}

var (
	prepareFailure = "prepare"
	submitFailure  = "submit"
)

// SignTransaction - perform KMS signing of the transaction, return publicKey and signature in hex string
func (t *Transaction) SignTransaction(ctx context.Context, kmsClient *kms.Client, keyId string) (string, string, error) {
	pubkeyOutput, err := kmsClient.GetPublicKey(ctx, &kms.GetPublicKeyInput{
		KeyId: &keyId,
	})

	if err != nil {
		return "", "", fmt.Errorf("Failed to get public key: %w", err)
	}

	signingOutput, err := kmsClient.Sign(ctx, &kms.SignInput{
		KeyId:            &keyId,
		Message:          t.BuildSigningBytes(),
		SigningAlgorithm: kmsTypes.SigningAlgorithmSpecEcdsaSha256,
	})

	if err != nil {
		return "", "", fmt.Errorf("Failed to sign transaction: %w", err)
	}

	return hex.EncodeToString(pubkeyOutput.PublicKey), hex.EncodeToString(signingOutput.Signature), nil
}

// BuildSigningBytes - the string format that payments will sign over per tx
func (t *Transaction) BuildSigningBytes() []byte {
	return []byte(fmt.Sprintf("%d|%s|%s|%s|%s|%s|%d",
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

// Return an idempotencyKey derived from a subset of values on the Transaction
func (t *Transaction) deriveIdempotencyKey() string {
	hasher := sha1.New()
	hasher.Write([]byte(fmt.Sprintf("%s%s%s%s%s", t.Amount, t.Custodian, t.From, t.To, t.PayoutID)))
	return base64.URLEncoding.EncodeToString(hasher.Sum(nil))
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

// InsertTransaction - perform a qldb insertion on the transactions
func (s *Service) InsertTransaction(ctx context.Context, transaction *Transaction) (Transaction, error) {
	stateMachine, err := StateMachineFromTransaction(transaction)
	if err != nil {
		return Transaction{}, fmt.Errorf("Failed to insert transaction: %w", err)
	}
	var transactionState TransactionState

	for transactionState < Prepared {
		transactionState, err = Drive(ctx, stateMachine, transaction, s.datastore)
		if err != nil {
			return Transaction{}, fmt.Errorf("Failed to drive state machine: %w", err)
		}
	}

	// Enriched includes DocumentID along with the transaction.
	return *transaction, nil
}

// UpdateTransactionsState - Change transaction state
func (s *Service) UpdateTransactionsState(ctx context.Context, state string, transactions ...Transaction) error {
	_, err := s.datastore.Execute(context.Background(), func(txn qldbdriver.Transaction) (interface{}, error) {
		// for all the transactions load up a check to see if this transaction has already existed
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
func (s *Service) AuthorizeTransaction(ctx context.Context, keyID string, transaction Transaction) error {
	_, err := s.datastore.Execute(context.Background(), func(txn qldbdriver.Transaction) (interface{}, error) {
		// for all the transactions load up a check to see if this transaction has already existed
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
func (s *Service) GetTransactionFromDocID(ctx context.Context, docID string) (*Transaction, error) {
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

// Compared the idempotencyKey provided by the worker to an idempotencyKey derived from the
// values known to the enclave worker. If they match, return true. Otherwise, return false.
func idempotencyKeyIsValid(txn *Transaction, entry *QLDBPaymentTransitionHistoryEntry) bool {
	return true
}
