package payments

import (
	"testing"

	"github.com/amazon-ion/ion-go/ion"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestIdempotencyKeyGeneration(t *testing.T) {
	to, err := uuid.Parse("683bc9ba-497a-47a5-9587-3bd03fd722bd")
	from, err := uuid.Parse("af68d02a-907f-4e9a-8f74-b54c7629412b")
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
	assert.Equal(t, transaction.deriveIdempotencyKey(), "inWrgd0F_tFjI_thCMPDmgxGeUM=")
}
