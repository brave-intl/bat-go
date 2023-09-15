package payments

import (
	"testing"

	"github.com/amazon-ion/ion-go/ion"
	"github.com/google/uuid"
	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"
)

func TestIdempotencyKeyGeneration(t *testing.T) {
	namespaceUUID, err := uuid.Parse("7478bd8a-2247-493d-b419-368f1a1d7a6c")
	must.Equal(t, nil, err)

	transaction := AuthenticatedPaymentState{
		PaymentDetails: PaymentDetails{
			Amount:    ion.MustParseDecimal("12.234"),
			To:        "683bc9ba-497a-47a5-9587-3bd03fd722bd",
			From:      "af68d02a-907f-4e9a-8f74-b54c7629412b",
			Custodian: "uphold",
			PayoutID:            "78910",
		},
		Status:               Prepared,
		documentID:          "1234",
	}
	should.Equal(
		t,
		transaction.generateIdempotencyKey(namespaceUUID).String(),
		"5f07afb9-aac0-5dba-9378-5c3fc34b6ff2",
	)
}
