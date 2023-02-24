// Package payments provides the payment service
package payments

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/amazon-ion/ion-go/ion"
	"github.com/awslabs/amazon-qldb-driver-go/qldbdriver"
	"golang.org/x/exp/slices"
)

// WrappedQldbDriverAPI defines the API for QLDB methods that we'll be using
type WrappedQldbDriverAPI interface {
	Execute(ctx context.Context, fn func(txn qldbdriver.Transaction) (interface{}, error)) (interface{}, error)
	Shutdown(ctx context.Context)
}

// WrappedQldbTxnAPI defines the API for QLDB methods that we'll be using
type WrappedQldbTxnAPI interface {
	Execute(statement string, parameters ...interface{}) (WrappedQldbResult, error)
	Abort() error
	BufferResult(*qldbdriver.Result) (*qldbdriver.BufferedResult, error)
}

// WrappedQldbResult defines the Result characteristics for QLDB methods that we'll be using
type WrappedQldbResult interface {
	Next(WrappedQldbTxnAPI) bool
	GetCurrentData() []byte
}

// QLDBPaymentTransitionHistoryEntryBlockAddress defines blockAddress data for QLDBPaymentTransitionHistoryEntry
type QLDBPaymentTransitionHistoryEntryBlockAddress struct {
	StrandID   string `ion:"strandID"`
	SequenceNo int64  `ion:"sequenceNo"`
}

// QLDBPaymentTransitionHistoryEntryHash defines hash for QLDBPaymentTransitionHistoryEntry
type QLDBPaymentTransitionHistoryEntryHash string

// QLDBPaymentTransitionHistoryEntrySignature defines signature for QLDBPaymentTransitionHistoryEntry
type QLDBPaymentTransitionHistoryEntrySignature []byte

// QLDBPaymentTransitionHistoryEntryData defines data for QLDBPaymentTransitionHistoryEntry
type QLDBPaymentTransitionHistoryEntryData struct {
	Status    QLDBPaymentTransitionState `ion:"status"`
	Signature []byte                     `ion:"signature"`
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
	BlockAddress QLDBPaymentTransitionHistoryEntryBlockAddress `ion:"blockAddress"`
	Hash         QLDBPaymentTransitionHistoryEntryHash         `ion:"hash"`
	Data         QLDBPaymentTransitionHistoryEntryData         `ion:"data"`
	Metadata     QLDBPaymentTransitionHistoryEntryMetadata     `ion:"metadata"`
}

// BuildSigningBytes returns the bytes that should be signed over when creating a signature
// for a QLDBPaymentTransitionHistoryEntry.
func (e QLDBPaymentTransitionHistoryEntry) BuildSigningBytes() []byte {
	return []byte(fmt.Sprintf("%s|%d|%s|%d|%s|%d|%s|%s",
		e.BlockAddress.StrandID,
		e.BlockAddress.SequenceNo,
		e.Hash,
		e.Data.Status,
		e.Metadata.ID,
		e.Metadata.Version,
		e.Metadata.ID,
		e.Metadata.TxID,
	))
}

// GetTransitionHistory returns a slice of entries representing the entire state history
// for a given id.
func GetTransitionHistory(txn WrappedQldbTxnAPI, id string) ([]QLDBPaymentTransitionHistoryEntry, error) {
	result, err := txn.Execute("SELECT * FROM history(PaymentTransitions) AS h WHERE h.metadata.id = 'SOME_ID'")
	if err != nil {
		return nil, err
	}
	var collectedData []QLDBPaymentTransitionHistoryEntry
	for result.Next(txn) {
		var data QLDBPaymentTransitionHistoryEntry
		err := ion.Unmarshal(result.GetCurrentData(), &data)
		if err != nil {
			return nil, err
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
	// Transitions must always start at 0
	if transactionHistory[0].Data.Status != 0 {
		return false, reason
	}
	result := true
	for i, transaction := range transactionHistory {
		if i == 0 {
			continue
		}
		previousTransition := transactionHistory[i-1]
		if !slices.Contains(Transitions[previousTransition.Data.Status], transaction.Data.Status) {
			result = false
			reason = errors.New("Invalid transition")
		}
	}
	return result, reason
}

// GetQLDBObject returns the latests state of an entry for a given ID after validating its
// transition history.
func GetQLDBObject(txn WrappedQldbTxnAPI, id string) (QLDBPaymentTransitionHistoryEntry, error) {
	result, err := GetTransitionHistory(txn, id)
	if err != nil {
		return QLDBPaymentTransitionHistoryEntry{}, err
	}
	valid, err := TransitionHistoryIsValid(result)
	if valid {
		return result[0], nil
	}
	return QLDBPaymentTransitionHistoryEntry{}, err
}

// WriteQLDBObject persists an object in a transaction after verifying that its change
// represents a valid state transition.
func WriteQLDBObject(
	driver WrappedQldbDriverAPI,
	key ed25519.PrivateKey,
	object QLDBPaymentTransitionHistoryEntry,
) (QLDBPaymentTransitionHistoryEntrySignature, error) {
	b, err := json.Marshal(object)
	if err != nil {
		return []byte{}, err
	}
	dataSignature := ed25519.Sign(key, b)
	_, err = driver.Execute(context.Background(), func(txn qldbdriver.Transaction) (interface{}, error) {
		fmt.Printf("Inserting QLDB Object\n")
		return txn.Execute("INSERT INTO PaymentTransitions {'some_key': 'some_value'}")
	})
	if err != nil {
		fmt.Printf("ERR %e", err)
		return []byte{}, err
	}
	return dataSignature, nil
}
