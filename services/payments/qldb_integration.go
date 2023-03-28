// Package payments provides the payment service
package payments

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/amazon-ion/ion-go/ion"
	"github.com/aws/aws-sdk-go-v2/service/qldb"
	qldbTypes "github.com/aws/aws-sdk-go-v2/service/qldb/types"
	"github.com/awslabs/amazon-qldb-driver-go/qldbdriver"
	"golang.org/x/exp/slices"
)

// wrappedQldbDriverAPI defines the API for QLDB methods that we'll be using
type wrappedQldbDriverAPI interface {
	Execute(ctx context.Context, fn func(txn qldbdriver.Transaction) (interface{}, error)) (interface{}, error)
	Shutdown(ctx context.Context)
}

type wrappedQldbSdkClient interface {
	New() qldb.Client
	GetDigest(
		ctx context.Context,
		params *qldb.GetDigestInput,
		optFns ...func(*qldb.Options),
	) (*qldb.GetDigestOutput, error)
	GetRevision(
		ctx context.Context,
		params *qldb.GetRevisionInput,
		optFns ...func(*qldb.Options),
	) (*qldb.GetRevisionOutput, error)
}

// wrappedQldbTxnAPI defines the API for QLDB methods that we'll be using
type wrappedQldbTxnAPI interface {
	Execute(statement string, parameters ...interface{}) (wrappedQldbResult, error)
	Abort() error
	BufferResult(*qldbdriver.Result) (*qldbdriver.BufferedResult, error)
}

// wrappedQldbResult defines the Result characteristics for QLDB methods that we'll be using
type wrappedQldbResult interface {
	Next(wrappedQldbTxnAPI) bool
	GetCurrentData() []byte
}

// qldbPaymentTransitionHistoryEntryBlockAddress defines blockAddress data for QLDBPaymentTransitionHistoryEntry
type qldbPaymentTransitionHistoryEntryBlockAddress struct {
	StrandID   string `ion:"strandID"`
	SequenceNo int64  `ion:"sequenceNo"`
}

// QLDBPaymentTransitionHistoryEntryHash defines hash for QLDBPaymentTransitionHistoryEntry
type QLDBPaymentTransitionHistoryEntryHash string

// QLDBPaymentTransitionHistoryEntrySignature defines signature for QLDBPaymentTransitionHistoryEntry
type QLDBPaymentTransitionHistoryEntrySignature []byte

// QLDBPaymentTransitionData represents the data for a transaction. It is stored in QLDB
// in a serialized format and needs to be separately deserialized from the QLDB ion
// deserialization.
type QLDBPaymentTransitionData struct {
	Status QLDBPaymentTransitionState `ion:"status"`
}

// QLDBPaymentTransitionHistoryEntryData defines data for QLDBPaymentTransitionHistoryEntry
type QLDBPaymentTransitionHistoryEntryData struct {
	Signature []byte `ion:"signature"`
	Data      []byte `ion:data`
}

// QLDBPaymentTransitionHistoryEntryMetadata defines metadata for QLDBPaymentTransitionHistoryEntry
type QLDBPaymentTransitionHistoryEntryMetadata struct {
	ID      string    `ion:"id"`
	Version int64     `ion:"version"`
	TxTime  time.Time `ion:"txTime"`
	TxID    string    `ion:"txId"`
}

// QLDBPaymentTransitionHistoryEntry defines top level entry for a QLDB transaction
type QLDBPaymentTransitionHistoryEntry struct {
	BlockAddress qldbPaymentTransitionHistoryEntryBlockAddress `ion:"blockAddress"`
	Hash         QLDBPaymentTransitionHistoryEntryHash         `ion:"hash"`
	Data         QLDBPaymentTransitionHistoryEntryData         `ion:"data"`
	Metadata     QLDBPaymentTransitionHistoryEntryMetadata     `ion:"metadata"`
}

// BuildSigningBytes returns the bytes that should be signed over when creating a signature
// for a QLDBPaymentTransitionHistoryEntry.
func (e QLDBPaymentTransitionHistoryEntry) BuildSigningBytes() ([]byte, error) {
	marshaled, err := ion.MarshalBinary(e)
	if err != nil {
		return nil, fmt.Errorf("Ion unmarshal failed: %w", err)
	}

	return marshaled, nil
}

// ValueHolder converts a QLDBPaymentTransitionHistoryEntry into a QLDB SDK ValueHolder
func (b qldbPaymentTransitionHistoryEntryBlockAddress) ValueHolder() *qldbTypes.ValueHolder {
	stringValue := fmt.Sprintf("{strandId:\"%s\",sequenceNo:%d}", b.StrandID, b.SequenceNo)
	return &qldbTypes.ValueHolder{
		IonText: &stringValue,
	}
}

// GetTransitionHistory returns a slice of entries representing the entire state history
// for a given id.
func GetTransitionHistory(txn wrappedQldbTxnAPI, id string) ([]QLDBPaymentTransitionHistoryEntry, error) {
	result, err := txn.Execute("SELECT * FROM history(PaymentTransitions) AS h WHERE h.metadata.id = ?", id)
	if err != nil {
		return nil, fmt.Errorf("QLDB transaction failed: %w", err)
	}
	var collectedData []QLDBPaymentTransitionHistoryEntry
	for result.Next(txn) {
		var data QLDBPaymentTransitionHistoryEntry
		err := ion.Unmarshal(result.GetCurrentData(), &data)
		if err != nil {
			return nil, fmt.Errorf("Ion unmarshal failed: %w", err)
		}
		collectedData = append(collectedData, data)
	}
	if len(collectedData) > 0 {
		return collectedData, nil
	}
	return nil, nil
}

// TransitionHistoryIsValid returns whether a slice of entries representing the entire state
// history for a given id include exculsively valid transitions.
func TransitionHistoryIsValid(transactionHistory []QLDBPaymentTransitionHistoryEntry) (bool, error) {
	var reason error
	for i, transaction := range transactionHistory {
		var transactionData QLDBPaymentTransitionData
		json.Unmarshal(transaction.Data.Data, transactionData)
		// Transitions must always start at 0
		if i == 0 {
			if transactionData.Status != 0 {
				return false, errors.New("Initial state is not valid")
			} else {
				continue
			}
		}
		var previousTransitionData QLDBPaymentTransitionData
		json.Unmarshal(transactionHistory[i-1].Data.Data, previousTransitionData)
		if !slices.Contains(Transitions[previousTransitionData.Status], transactionData.Status) {
			return false, errors.New("Invalid transition")
		}
	}
	return true, reason
}

func RevisionValidInTree(
	ctx context.Context,
	client wrappedQldbSdkClient,
	transaction QLDBPaymentTransitionHistoryEntry,
) (bool, error) {
	ledgerName := "LEDGER_NAME"
	digest, err := client.GetDigest(ctx, &qldb.GetDigestInput{Name: &ledgerName})
	if err != nil {
		return false, fmt.Errorf("Failed to get digest: %w", err)
	}
	revision, err := client.GetRevision(ctx, &qldb.GetRevisionInput{
		BlockAddress:     transaction.BlockAddress.ValueHolder(),
		DocumentId:       &transaction.Metadata.ID,
		Name:             &ledgerName,
		DigestTipAddress: digest.DigestTipAddress,
	})
	if err != nil {
		return false, fmt.Errorf("Failed to get revision: %w", err)
	}
	var (
		hashes      [][]byte
		currentHash [32]byte
	)
	err = ion.Unmarshal([]byte(*revision.Proof.IonText), &hashes)
	if err != nil {
		return false, fmt.Errorf("Failed to unmarshal revision proof: %w", err)
	}
	for hash := range hashes {
		hashable := []byte(fmt.Sprintf("%s", hash))
		// @TODO We need to hash two values together here, not just each single hash
		currentHash = sha256.Sum256(hashable)
	}
	if len(currentHash) != len(digest.Digest) {
		return false, nil
	}
	if string(currentHash[:]) == string(digest.Digest) {
		return true, nil
	}
	return false, nil
}

// GetQLDBObject returns the latests state of an entry for a given ID after validating its
// transition history.
func GetQLDBObject(txn wrappedQldbTxnAPI, id string) (QLDBPaymentTransitionHistoryEntry, error) {
	result, err := GetTransitionHistory(txn, id)
	if err != nil {
		return QLDBPaymentTransitionHistoryEntry{}, fmt.Errorf("Failed to get transition history: %w", err)
	}
	valid, err := TransitionHistoryIsValid(result)
	if valid {
		return result[0], nil
	}
	return QLDBPaymentTransitionHistoryEntry{}, fmt.Errorf("Invalid transition history: %w", err)
}

// WriteQLDBObject persists an object in a transaction after verifying that its change
// represents a valid state transition.
func WriteQLDBObject(
	driver wrappedQldbDriverAPI,
	key ed25519.PrivateKey,
	object QLDBPaymentTransitionHistoryEntry,
) (QLDBPaymentTransitionHistoryEntrySignature, error) {
	b, err := json.Marshal(object)
	if err != nil {
		return []byte{}, fmt.Errorf("JSON marshal failed: %w", err)
	}
	dataSignature := ed25519.Sign(key, b)
	_, err = driver.Execute(context.Background(), func(txn qldbdriver.Transaction) (interface{}, error) {
		return txn.Execute("INSERT INTO PaymentTransitions {'some_key': 'some_value'}")
	})
	if err != nil {
		return []byte{}, fmt.Errorf("QLDB execution failed: %w", err)
	}
	return dataSignature, nil
}
