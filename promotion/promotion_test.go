package promotion

import (
	"context"
	"testing"

	"github.com/brave-intl/bat-go/wallet"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestGetAvailablePromotions(t *testing.T) {
	pg, err := NewPostgres("", true)
	assert.NoError(t, err, "failed to connect to db")
	service, err := InitService(pg)
	assert.NoError(t, err, "failed to create service")

	publicKey := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="
	walletUUID := uuid.NewV4()
	walletID := walletUUID.String()
	w := &wallet.Info{
		ID:         walletID,
		Provider:   "uphold",
		ProviderID: uuid.NewV4().String(),
		PublicKey:  publicKey,
	}
	assert.NoError(t, pg.InsertWallet(w), "Failed to insert wallet")

	_, err = pg.CreatePromotion("ugp", 1, decimal.NewFromFloat(25.0), "")
	assert.NoError(t, err, "Failed to insert promotion")

	_, err = service.GetAvailablePromotions(
		context.Background(),
		&walletUUID,
		"",
		false,
	)
	assert.NoError(t, err, "Failed to get available promotions")
	// assert.Equal(t, 1, len(promotions), "should have one promotion")
	// assert.Equal(t, promotion.ID, promotions[0].ID, "id should match the one inserted above")

	// change the cooldown time to fail
	walletCooldown = 1000

	_, err = service.GetAvailablePromotions(
		context.Background(),
		&walletUUID,
		"",
		false,
	)
	assert.Error(t, err, "Should fail to get available promotions")
	// put the value back
	walletCooldown = 0
}
