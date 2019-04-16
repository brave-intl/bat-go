package grant

import (
	"context"

	"github.com/brave-intl/bat-go/wallet"
	"github.com/pressly/lg"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	claimKeyFormat = "grant:%s:claim"
)

// ClaimGrantRequest is a request to claim a grant
type ClaimGrantRequest struct {
	WalletInfo wallet.Info `json:"wallet" valid:"required"`
}

// Claim registers a claim on behalf of a user wallet to a particular Grant.
// Registered claims are enforced by RedeemGrantsRequest.Verify.
func (service *Service) Claim(ctx context.Context, req *ClaimGrantRequest, grantID string) error {
	log := lg.Log(ctx)

	err := service.datastore.ClaimGrantIDForWallet(grantID, req.WalletInfo)
	if err != nil {
		log.Error("Attempt to claim previously claimed grant!")
		return err
	}
	claimedGrantsCounter.With(prometheus.Labels{}).Inc()

	return nil
}
