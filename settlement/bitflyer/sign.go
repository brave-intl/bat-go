package bitflyersettlement

import (
	"errors"

	"github.com/brave-intl/bat-go/settlement"
	bitflyerclient "github.com/brave-intl/bat-go/utils/clients/bitflyer"
)

// SettlementRequest holds the api key and settlements to be settled on bitflyer
type SettlementRequest struct {
	APIKey       string                              `json:"api_key"`
	Transactions map[string][]settlement.Transaction `json:"transactions"`
}

// GroupSettlements groups settlements under a single provider id so that we can impose limits based on price
// no signing here, just grouping settlements under a single deposit id
func GroupSettlements(
	token string,
	settlements *[]settlement.Transaction,
) (*SettlementRequest, error) {
	if len(token) == 0 {
		return nil, errors.New("a client token was missing during the bitflyer settlement signing process")
	}

	grouped := make(map[string][]settlement.Transaction)
	for _, payout := range *settlements {
		id := bitflyerclient.GenerateTransferID(&payout)
		grouped[id] = append(grouped[id], payout)
	}

	return &SettlementRequest{
		APIKey:       token,
		Transactions: grouped,
	}, nil
}
