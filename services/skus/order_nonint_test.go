package skus

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"

	appctx "github.com/brave-intl/bat-go/libs/context"
	macarooncmd "github.com/brave-intl/bat-go/tools/macaroon/cmd"

	"github.com/brave-intl/bat-go/services/skus/model"
)

// TestCreateOrderItemFromMacaroonQuantityBounds guards against zero or negative
// quantity order items: a non-positive quantity zeroes out the order subtotal,
// which marks the order paid without any payment and mints premium credentials.
func TestCreateOrderItemFromMacaroonQuantityBounds(t *testing.T) {
	c := macarooncmd.Caveats{
		"sku":                     "sku",
		"price":                   "5.01",
		"description":             "coffee",
		"currency":                "usd",
		"credential_type":         "time_bound",
		"allowed_payment_methods": "stripe",
	}

	token := macarooncmd.Token{
		ID:                "id",
		Version:           2,
		Location:          "brave.com",
		FirstPartyCaveats: []macarooncmd.Caveats{c},
	}

	sku, err := token.Generate("testing123")
	must.NoError(t, err)

	skuMap["development"][sku] = true

	svc := &Service{}
	ctx := context.WithValue(context.Background(), appctx.EnvironmentCTXKey, "development")

	t.Run("zero quantity is rejected", func(t *testing.T) {
		_, _, _, err := svc.CreateOrderItemFromMacaroon(ctx, sku, 0)
		must.ErrorIs(t, err, model.ErrInvalidQuantity)
	})

	t.Run("negative quantity is rejected", func(t *testing.T) {
		_, _, _, err := svc.CreateOrderItemFromMacaroon(ctx, sku, -1)
		must.ErrorIs(t, err, model.ErrInvalidQuantity)
	})

	t.Run("quantity one is the smallest valid", func(t *testing.T) {
		orderItem, _, _, err := svc.CreateOrderItemFromMacaroon(ctx, sku, 1)
		must.NoError(t, err)

		should.Equal(t, 1, orderItem.Quantity)
		should.True(t, orderItem.Subtotal.Equal(decimal.RequireFromString("5.01")))
	})
}
