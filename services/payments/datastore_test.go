package payments

import (
	"testing"

	"github.com/amazon-ion/ion-go/ion"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestIdempotencyKeyGeneration(t *testing.T) {
	to, err := uuid.Parse("")
	from, err := uuid.Parse("")
	if err != nil {
		panic("failed to parse test UUIDs")
	}
	transaction := Transaction{
		IdempotencyKey:      "",
		Amount:              ion.MustParseDecimal("12.234"),
		To:                  &to,
		From:                &from,
		Custodian:           "uphold",
		State:               Initialized,
		DocumentID:          "1234",
		AttestationDocument: "4567",
		PayoutID:            "78910",
		Signature:           "",
		PublicKey:           "",
	}
	assert.Equal(t, transaction.deriveIdempotencyKey(), "")
}

func Test_fromIonDecimal(t *testing.T) {
	type args struct {
		v *ion.Decimal
	}
	var tests []struct {
		name string
		args args
		want *decimal.Decimal
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, fromIonDecimal(tt.args.v), "fromIonDecimal(%v)", tt.args.v)
		})
	}
}

func TestIdempotencyKeyIsValid(t *testing.T) {
	type args struct {
		txn   *Transaction
		entry *QLDBPaymentTransitionHistoryEntry
	}
	var tests []struct {
		name string
		args args
		want bool
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, idempotencyKeyIsValid(tt.args.txn, tt.args.entry), "idempotencyKeyIsValid(%v, %v)", tt.args.txn, tt.args.entry)
		})
	}
}

func TestToIonDecimal(t *testing.T) {
	type args struct {
		v *decimal.Decimal
	}
	var tests []struct {
		name string
		args args
		want *ion.Decimal
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, toIonDecimal(tt.args.v), "toIonDecimal(%v)", tt.args.v)
		})
	}
}
