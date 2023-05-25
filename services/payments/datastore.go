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
	"github.com/shopspring/decimal"
	"golang.org/x/exp/slices"

	"github.com/amazon-ion/ion-go/ion"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmsTypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/awslabs/amazon-qldb-driver-go/v3/qldbdriver"
	"github.com/google/uuid"
)

// Transaction - the main type explaining a transaction, type used for qldb via ion
type Transaction struct {
	ID                  *uuid.UUID       `json:"idempotencyKey,omitempty" ion:"idempotencyKey" valid:"required"`
	Amount              *ion.Decimal     `json:"amount" ion:"amount" valid:"required"`
	To                  *uuid.UUID       `json:"to,omitempty" ion:"to" valid:"required"`
	From                *uuid.UUID       `json:"from,omitempty" ion:"from" valid:"required"`
	Custodian           string           `json:"custodian,omitempty" ion:"custodian" valid:"in(uphold|gemini|bitflyer)"`
	State               TransactionState `json:"state" ion:"state"`
	DocumentID          string           `json:"documentId,omitempty" ion:"id"`
	AttestationDocument string           `json:"attestation,omitempty" ion:"-"`
	PayoutID            string           `json:"payoutId" valid:"required"`
	Signature           string           `json:"-" ion:"signature"` // KMS signature only enclave can sign
	PublicKey           string           `json:"-" ion:"publicKey"` // KMS signature only enclave can sign
	Currency            string           `json:"-" ion:"currency"`
	DryRun              *string          `json:"dryRun" ion:"-"` // determines dry-run
}

// qldbPaymentTransitionHistoryEntryBlockAddress defines blockAddress data for qldbPaymentTransitionHistoryEntry
type qldbPaymentTransitionHistoryEntryBlockAddress struct {
	StrandID   string `ion:"strandId"`
	SequenceNo int64  `ion:"sequenceNo"`
}

// qldbPaymentTransitionHistoryEntryHash defines hash for qldbPaymentTransitionHistoryEntry
type qldbPaymentTransitionHistoryEntryHash string

// qldbPaymentTransitionHistoryEntrySignature defines signature for qldbPaymentTransitionHistoryEntry
type qldbPaymentTransitionHistoryEntrySignature []byte

// qldbPaymentTransitionHistoryEntryData defines data for qldbPaymentTransitionHistoryEntry
type qldbPaymentTransitionHistoryEntryData struct {
	Signature      []byte     `ion:"signature"`
	Data           []byte     `ion:"data"`
	IdempotencyKey *uuid.UUID `ion:"idempotencyKey"`
}

// qldbPaymentTransitionHistoryEntryMetadata defines metadata for qldbPaymentTransitionHistoryEntry
type qldbPaymentTransitionHistoryEntryMetadata struct {
	ID      string    `ion:"id"`
	TxID    string    `ion:"txId"`
	TxTime  time.Time `ion:"txTime"`
	Version int64     `ion:"version"`
}

// qldbPaymentTransitionHistoryEntry defines top level entry for a QLDB transaction
type qldbPaymentTransitionHistoryEntry struct {
	BlockAddress qldbPaymentTransitionHistoryEntryBlockAddress `ion:"blockAddress"`
	Hash         qldbPaymentTransitionHistoryEntryHash         `ion:"hash"`
	Data         qldbPaymentTransitionHistoryEntryData         `ion:"data"`
	Metadata     qldbPaymentTransitionHistoryEntryMetadata     `ion:"metadata"`
}

func (e *qldbPaymentTransitionHistoryEntry) toTransaction() (*Transaction, error) {
	var txn Transaction
	err := ion.Unmarshal(e.Data.Data, &txn)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal record data for conversion from qldbPaymentTransitionHistoryEntry to Transaction: %w", err)
	}
	return &txn, nil
}

// GenerateIdempotencyKey returns a UUID v5 ID if the ID on the Transaction matches its expected value. Otherwise, it returns
// an error
func (t *Transaction) GenerateIdempotencyKey(namespace uuid.UUID) (*uuid.UUID, error) {
	generatedIdempotencyKey := t.generateIdempotencyKey(namespace)
	if generatedIdempotencyKey != *t.ID {
		return nil, fmt.Errorf("ID does not match transaction fields: have %s, want %s", *t.ID, generatedIdempotencyKey)
	}

	return t.ID, nil
}

// SetIdempotencyKey assigns a UUID v5 value to Transaction.ID
func (t *Transaction) SetIdempotencyKey(namespace uuid.UUID) error {
	generatedIdempotencyKey := t.generateIdempotencyKey(namespace)
	t.ID = &generatedIdempotencyKey
	return nil
}

var (
	prepareFailure = "prepare"
	submitFailure  = "submit"
)

// SignTransaction - perform KMS signing of the transaction, return publicKey and signature in hex string
func (t *Transaction) SignTransaction(ctx context.Context, kmsClient wrappedKMSClient, keyID string) (string, string, error) {
	pubkeyOutput, err := kmsClient.GetPublicKey(ctx, &kms.GetPublicKeyInput{
		KeyId: &keyID,
	})

	if err != nil {
		return "", "", fmt.Errorf("Failed to get public key: %w", err)
	}

	marshaled, err := ion.MarshalBinary(t)
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

func (t *Transaction) generateIdempotencyKey(namespace uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(namespace, []byte(fmt.Sprintf("%s%s%s%s%s%s", t.To, t.From, t.Currency, t.Amount, t.Custodian, t.PayoutID)))
}

func (t *Transaction) getIdempotencyKey() *uuid.UUID {
	return t.ID
}

func (t *Transaction) nextStateValid(nextState TransactionState) bool {
	if t.State == nextState {
		return true
	}
	// New transaction state should be present in the list of valid next states for the current state.
	if !slices.Contains(t.State.GetValidTransitions(), nextState) {
		return false
	}
	return true
}

func toIonDecimal(v *decimal.Decimal) *ion.Decimal {
	// @TODO: Do we want to panic here?
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

func (s *Service) progressTransacton(ctx context.Context, transaction *Transaction) (Transaction, error) {
	stateMachine, err := StateMachineFromTransaction(transaction, s)
	if err != nil {
		return Transaction{}, fmt.Errorf("failed to insert transaction: %w", err)
	}

	// Only drive the Transaction into the targetState
	transaction, err = Drive(ctx, stateMachine)
	if err != nil {
		return Transaction{}, fmt.Errorf("failed to drive state machine: %w", err)
	}

	// Enriched includes DocumentID along with the transaction.
	return *transaction, nil
}

// PrepareTransaction - perform a qldb insertion on the transaction
func (s *Service) PrepareTransaction(ctx context.Context, transaction *Transaction) (Transaction, error) {
	return s.progressTransacton(ctx, transaction)
}

// AuthorizeTransaction - Add an Authorization for the Transaction
func (s *Service) AuthorizeTransaction(ctx context.Context, keyID string, transaction Transaction) error {
	_, err := s.datastore.Execute(context.Background(), func(txn qldbdriver.Transaction) (interface{}, error) {
		// for all the transactions load up a check to see if this transaction has already existed
		// or not, then perform the insertion of the records.
		auth := map[string]string{
			"keyID":      keyID,
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
	transaction, err = s.progressTransacton(ctx, &transaction)
	if err != nil {
		return fmt.Errorf("failed to progress transaction: %w", err)
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

// getQLDBObject returns the latest version of an entry for a given ID after doing all requisite validation
func (s *Service) getQLDBObject(
	ctx context.Context,
	qldbTransactionDriver wrappedQldbTxnAPI,
	txnID *uuid.UUID,
	namespace uuid.UUID,
) (*qldbPaymentTransitionHistoryEntry, error) {
	valid, result, err := transactionHistoryIsValid(ctx, qldbTransactionDriver, s.kmsSigningClient, txnID, namespace)
	if err != nil || !valid {
		return nil, fmt.Errorf("failed to validate transition history: %w", err)
	}
	// If no record was found, return nothing
	if result == nil {
		return nil, nil
	}
	merkleValid, err := revisionValidInTree(ctx, s.sdkClient, result)
	if err != nil {
		return nil, fmt.Errorf("failed to verify Merkle tree: %w", err)
	}
	if !merkleValid {
		return nil, fmt.Errorf("invalid Merkle tree for record: %#v", result)
	}
	return result, nil
}

// GetTransactionByID returns the latest version of a record from QLDB if it exists, after doing all requisite validation
func (s *Service) GetTransactionByID(ctx context.Context, id *uuid.UUID) (*Transaction, error) {
	namespace, ok := ctx.Value(serviceNamespaceContextKey{}).(uuid.UUID)
	if !ok {
		return nil, fmt.Errorf("Failed to get UUID namespace from context")
	}
	data, err := s.datastore.Execute(ctx, func(txn qldbdriver.Transaction) (interface{}, error) {
		entry, err := s.getQLDBObject(ctx, txn, id, namespace)
		if err != nil {
			return nil, fmt.Errorf("failed to get QLDB record: %w", err)
		}
		return entry, nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query QLDB: %w", err)
	}
	if data == nil {
		return nil, nil
	}
	assertedData, ok := data.(*qldbPaymentTransitionHistoryEntry)
	if !ok {
		return nil, fmt.Errorf("database response was the wrong type: %#v", data)
	}
	transaction, err := assertedData.toTransaction()
	if err != nil {
		return nil, fmt.Errorf("failed to convert record to Transaction: %w", err)
	}
	return transaction, nil
}

// getTransactionHistory returns a slice of entries representing the entire state history
// for a given id.
func getTransactionHistory(txn wrappedQldbTxnAPI, id *uuid.UUID) ([]qldbPaymentTransitionHistoryEntry, error) {
	result, err := txn.Execute("SELECT * FROM history(transactions) AS h WHERE h.metadata.id = ?", id)
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
func (s *Service) WriteTransaction(ctx context.Context, transaction *Transaction) (*Transaction, error) {
	namespace, ok := ctx.Value(serviceNamespaceContextKey{}).(uuid.UUID)
	if !ok {
		return nil, fmt.Errorf("Failed to get UUID namespace from context")
	}
	_, err := s.datastore.Execute(ctx, func(txn qldbdriver.Transaction) (interface{}, error) {
		// Determine if the transaction already exists or if it needs to be initialized. This call will do all necessary
		// record and history validation if they exist for this record
		record, err := s.getQLDBObject(ctx, txn, transaction.ID, namespace)
		if err != nil {
			return nil, fmt.Errorf("failed to query QLDB: %w", err)
		}
		transaction.PublicKey, transaction.Signature, err = transaction.SignTransaction(ctx, s.kmsSigningClient, s.kmsSigningKeyID)
		if err != nil {
			return nil, fmt.Errorf("failed to sign transaction: %w", err)
		}

		if record == nil {
			return txn.Execute("INSERT INTO transactions ?", transaction)
		}
		return txn.Execute("UPDATE transactions SET state = ? WHERE id = ?", transaction.State, transaction.ID)
	})
	if err != nil {
		return nil, fmt.Errorf("QLDB write execution failed: %w", err)
	}
	return transaction, nil
}
