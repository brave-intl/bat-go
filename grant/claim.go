package grant

import (
	"context"
	"errors"
	"fmt"

	"github.com/brave-intl/bat-go/datastore"
	"github.com/brave-intl/bat-go/utils"
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
func (req *ClaimGrantRequest) Claim(ctx context.Context, grantID string) error {
	log := lg.Log(ctx)

	kvDatastore, err := datastore.GetKvDatastore(ctx)
	if err != nil {
		return err
	}
	defer utils.PanicCloser(kvDatastore)

	_, err = kvDatastore.Set(
		fmt.Sprintf(claimKeyFormat, grantID),
		req.WalletInfo.ProviderID,
		ninetyDaysInSeconds,
		false,
	)
	if err != nil {
		log.Error("Attempt to claim previously claimed grant!")
		return errors.New("An existing claim to the grant already exists")
	}
	claimedGrantsCounter.With(prometheus.Labels{}).Inc()

	return nil
}

// GetClaimantID returns the providerID who has claimed a given grant
func GetClaimantID(kvDatastore datastore.KvDatastore, grantID string) (string, error) {
	return kvDatastore.Get(fmt.Sprintf(claimKeyFormat, grantID))
}
