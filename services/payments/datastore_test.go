package payments

import (
	"testing"

	"github.com/amazon-ion/ion-go/ion"
	"github.com/google/uuid"
	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"
)

func TestIdempotencyKeyGeneration(t *testing.T) {
	to, err := uuid.Parse("683bc9ba-497a-47a5-9587-3bd03fd722bd")
	must.Equal(t, nil, err)
	from, err := uuid.Parse("af68d02a-907f-4e9a-8f74-b54c7629412b")
	must.Equal(t, nil, err)
	id := uuid.New()
	transaction := Transaction{
		ID:                  &id,
		Amount:              ion.MustParseDecimal("12.234"),
		To:                  &to,
		From:                &from,
		Custodian:           "uphold",
		State:               Prepared,
		DocumentID:          "1234",
		AttestationDocument: "4567",
		PayoutID:            "78910",
		Signature:           "",
		PublicKey:           "",
	}
	should.Equal(t, transaction.generateIdempotencyKey(uuid.New()), "inWrgd0F_tFjI_thCMPDmgxGeUM=")
}
