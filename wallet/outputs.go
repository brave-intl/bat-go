package wallet

import (
	"github.com/brave-intl/bat-go/utils/altcurrency"
	walletutils "github.com/brave-intl/bat-go/utils/wallet"
)

const (
	InvalidCurrency = "invalid"
	BATCurrency     = "BAT"
	BTCCurrency     = "BTC"
	ETHCurrency     = "ETH"
	LTCCurrency     = "LTC"
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

// WalletResponseV3 - wallet creation response
type WalletResponseV3 struct {
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

func infoToResponseV3(info *walletutils.Info) WalletResponseV3 {
	var (
		linkingID        string
		anonymousAddress string
		altCurrency      string = convertAltCurrency(info.AltCurrency)
	)
	if info == nil {
		return WalletResponseV3{}
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

	resp := WalletResponseV3{
		PaymentID:   info.ID,
		AltCurrency: altCurrency,
		PublicKey:   info.PublicKey,
		WalletProvider: BraveProviderDetailsV3{
			Name: "brave",
			ID:   info.ProviderID,
		},
	}
	// if this is linked to uphold, add the default account provider
	if info.Provider == "uphold" {
		resp.DepositAccountProvider = UpholdProviderDetailsV3{
			Name:             info.Provider,
			LinkingID:        linkingID,
			AnonymousAddress: anonymousAddress,
		}
	}
	return resp
}
