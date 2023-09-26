package payments

import (
	"testing"

	. "github.com/brave-intl/bat-go/libs/payments"
	"github.com/shopspring/decimal"
	should "github.com/stretchr/testify/assert"
)

func TestIdempotencyKeyGeneration(t *testing.T) {
	transaction := AuthenticatedPaymentState{
		PaymentDetails: PaymentDetails{
			Amount:    decimal.NewFromFloat(12.234),
			To:        "683bc9ba-497a-47a5-9587-3bd03fd722bd",
			From:      "af68d02a-907f-4e9a-8f74-b54c7629412b",
			Custodian: "uphold",
			PayoutID:            "78910",
		},
		Status:               Prepared,
		DocumentID:          "1234",
	}
	should.Equal(
		t,
		transaction.GenerateIdempotencyKey().String(),
		"5f07afb9-aac0-5dba-9378-5c3fc34b6ff2",
	)
}
