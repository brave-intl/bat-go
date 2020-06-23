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

// ProviderDetailsV3 - details about the provider
type ProviderDetailsV3 struct {
	Name      string `json:"provider"`
	ID        string `json:"providerId"`
	LinkingID string `json:"providerLinkingId"`
}

// WalletResponseV3 - wallet creation response
type WalletResponseV3 struct {
	PaymentID   string            `json:"paymentId"`
	Provider    ProviderDetailsV3 `json:"provider"`
	AltCurrency string            `json:"altcurrency"`
	PublicKey   string            `json:"publicKey"`
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
		linkingID   string
		altCurrency string = convertAltCurrency(info.AltCurrency)
	)
	if info == nil {
		return WalletResponseV3{}
	}
	if info.ProviderLinkingID == nil {
		linkingID = ""
	}

	return WalletResponseV3{
		PaymentID:   info.ID,
		AltCurrency: altCurrency,
		PublicKey:   info.PublicKey,
		Provider: ProviderDetailsV3{
			Name:      info.Provider,
			ID:        info.ProviderID,
			LinkingID: linkingID,
		},
	}
}
