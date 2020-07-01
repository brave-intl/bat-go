package wallet

import (
	"github.com/brave-intl/bat-go/utils/altcurrency"
	walletutils "github.com/brave-intl/bat-go/utils/wallet"
)

const (
	// InvalidCurrency - wallet currency is invalid
	InvalidCurrency = "invalid"
	// BATCurrency - wallet currency is BAT
	BATCurrency = "BAT"
	// BTCCurrency - wallet currency is BTC
	BTCCurrency = "BTC"
	// ETHCurrency - wallet currency is ETH
	ETHCurrency = "ETH"
	// LTCCurrency - wallet currency is LTC
	LTCCurrency = "LTC"
)

// BraveProviderDetailsV3 - details about the provider
type BraveProviderDetailsV3 struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// UpholdProviderDetailsV3 - details about the provider
type UpholdProviderDetailsV3 struct {
	Name             string `json:"name"`
	LinkingID        string `json:"linkingId"`
	AnonymousAddress string `json:"anonymousAddress"`
}

// ResponseV3 - wallet creation response
type ResponseV3 struct {
	PaymentID              string      `json:"paymentId"`
	DepositAccountProvider interface{} `json:"depositAccountProvider,omitempty"`
	WalletProvider         interface{} `json:"walletProvider,omitempty"`
	AltCurrency            string      `json:"altcurrency"`
	PublicKey              string      `json:"publicKey"`
}

func convertAltCurrency(a *altcurrency.AltCurrency) string {
	if a == nil {
		return BATCurrency
	}
	switch *a {
	case altcurrency.BAT:
		return BATCurrency
	case altcurrency.BTC:
		return BTCCurrency
	case altcurrency.ETH:
		return ETHCurrency
	case altcurrency.LTC:
		return LTCCurrency
	default:
		return InvalidCurrency
	}
}

func infoToResponseV3(info *walletutils.Info) ResponseV3 {
	var (
		linkingID        string
		anonymousAddress string
		altCurrency      string = convertAltCurrency(info.AltCurrency)
	)
	if info == nil {
		return ResponseV3{}
	}

	if info.ProviderLinkingID == nil {
		linkingID = ""
	} else {
		linkingID = info.ProviderLinkingID.String()
	}

	if info.AnonymousAddress == nil {
		anonymousAddress = ""
	} else {
		anonymousAddress = info.AnonymousAddress.String()
	}

	resp := ResponseV3{
		PaymentID:   info.ID,
		AltCurrency: altCurrency,
		PublicKey:   info.PublicKey,
	}

	// if this is linked to uphold, add the default account provider
	if info.Provider == "uphold" {
		if anonymousAddress == "" {
			// no linked anon card
			resp.WalletProvider = BraveProviderDetailsV3{
				Name: "brave",
				ID:   info.ID,
			}
		} else {
			resp.WalletProvider = BraveProviderDetailsV3{
				Name: "uphold",
				ID:   info.ProviderID,
			}
		}
		if linkingID != "" {
			resp.DepositAccountProvider = UpholdProviderDetailsV3{
				Name:             info.Provider,
				LinkingID:        linkingID,
				AnonymousAddress: anonymousAddress,
			}
		}
	} else if info.Provider == "brave" {
		// no linked anon card
		resp.WalletProvider = BraveProviderDetailsV3{
			Name: "brave",
			ID:   info.ID,
		}
	} else {
		resp.WalletProvider = BraveProviderDetailsV3{
			Name: info.Provider,
			ID:   info.ProviderID,
		}
	}
	return resp
}
