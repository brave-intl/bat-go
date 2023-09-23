package payments

import (
	"encoding/json"
	"fmt"
	"time"

	qldbTypes "github.com/aws/aws-sdk-go-v2/service/qldb/types"
)

// QLDBPaymentTransitionHistoryEntryBlockAddress defines blockAddress data for
// qldbPaymentTransitionHistoryEntry.
type QLDBPaymentTransitionHistoryEntryBlockAddress struct {
	StrandID   string `ion:"strandId"`
	SequenceNo int64  `ion:"sequenceNo"`
}

// QLDBPaymentTransitionHistoryEntryHash defines hash for qldbPaymentTransitionHistoryEntry.
type QLDBPaymentTransitionHistoryEntryHash string

// qldbPaymentTransitionHistoryEntrySignature defines signature for
// qldbPaymentTransitionHistoryEntry.
type qldbPaymentTransitionHistoryEntrySignature []byte

// QLDBPaymentTransitionHistoryEntryMetadata defines metadata for qldbPaymentTransitionHistoryEntry
type QLDBPaymentTransitionHistoryEntryMetadata struct {
	ID      string    `ion:"id"`
	TxID    string    `ion:"txId"`
	TxTime  time.Time `ion:"txTime"`
	Version int64     `ion:"version"`
}

// QLDBPaymentTransitionHistoryEntry defines top level entry for a QLDB transaction.
type QLDBPaymentTransitionHistoryEntry struct {
	BlockAddress QLDBPaymentTransitionHistoryEntryBlockAddress `ion:"blockAddress"`
	Hash         QLDBPaymentTransitionHistoryEntryHash         `ion:"hash"`
	Data         PaymentState                                  `ion:"data"`
	Metadata     QLDBPaymentTransitionHistoryEntryMetadata     `ion:"metadata"`
}

// ValueHolder converts a QLDBPaymentTransitionHistoryEntry into a QLDB SDK ValueHolder.
func (b *QLDBPaymentTransitionHistoryEntryBlockAddress) ValueHolder() *qldbTypes.ValueHolder {
	stringValue := fmt.Sprintf("{strandId:\"%s\",sequenceNo:%d}", b.StrandID, b.SequenceNo)
	return &qldbTypes.ValueHolder{
		IonText: &stringValue,
	}
}

func (e *QLDBPaymentTransitionHistoryEntry) toTransaction() (*AuthenticatedPaymentState, error) {
	var txn AuthenticatedPaymentState
	err := json.Unmarshal(e.Data.UnsafePaymentState, &txn)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to unmarshal record data for conversion from qldbPaymentTransitionHistoryEntry to Transaction: %w",
			err,
		)
	}
	return &txn, nil
}
