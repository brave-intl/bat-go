package payments

import (
	"fmt"
	"time"

	qldbTypes "github.com/aws/aws-sdk-go-v2/service/qldb/types"
	paymentLib "github.com/brave-intl/bat-go/libs/payments"
)

// QLDBPaymentTransitionHistoryEntryBlockAddress defines blockAddress data for
// qldbPaymentTransitionHistoryEntry.
type QLDBPaymentTransitionHistoryEntryBlockAddress struct {
	StrandID   string `ion:"strandId"`
	SequenceNo int64  `ion:"sequenceNo"`
}

// QLDBPaymentTransitionHistoryEntryHash defines hash for qldbPaymentTransitionHistoryEntry.
type QLDBPaymentTransitionHistoryEntryHash []byte

/* TODO: unused
// qldbPaymentTransitionHistoryEntrySignature defines signature for qldbPaymentTransitionHistoryEntry.
type qldbPaymentTransitionHistoryEntrySignature []byte
*/

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
	Data         paymentLib.PaymentState                       `ion:"data"`
	Metadata     QLDBPaymentTransitionHistoryEntryMetadata     `ion:"metadata"`
}

// ValueHolder converts a QLDBPaymentTransitionHistoryEntry into a QLDB SDK ValueHolder.
func (b *QLDBPaymentTransitionHistoryEntryBlockAddress) ValueHolder() *qldbTypes.ValueHolder {
	stringValue := fmt.Sprintf("{strandId:\"%s\",sequenceNo:%d}", b.StrandID, b.SequenceNo)
	return &qldbTypes.ValueHolder{
		IonText: &stringValue,
	}
}

// SignableQLDBRecord is an interface that requires the ability for type to be turned into a
// SerializedQLDBRecord
type SignableQLDBRecord interface {
	ToSerializedQLDBRecord(pubkey string, signer paymentLib.Signator) (*SerializedQLDBRecord, error)
	GetID() string
}

// SerializedQLDBRecord is a generic representation of signed data for the service. It should only
// be used going into and out of QLDB and the data it contains should be otherwise dealt with in a
// structured format.
type SerializedQLDBRecord struct {
	Data      []byte `ion:"data"`
	ID        string `ion:"idempotencyKey"`
	PublicKey string `ion:"signingPublicKey"`
	Signature []byte `ion:"signature"`
}
