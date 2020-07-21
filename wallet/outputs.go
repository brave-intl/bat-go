package wallet

import (
	"github.com/brave-intl/bat-go/utils/altcurrency"
	walletutils "github.com/brave-intl/bat-go/utils/wallet"
	uuid "github.com/satori/go.uuid"
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

	//UpholdProvider - provider label for uphold wallets
	UpholdProvider = "uphold"
	//BraveProvider - provider label for brave wallets
	BraveProvider = "brave"
)

// ProviderDetailsV3 - details about the provider
type ProviderDetailsV3 struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	LinkingID        string `json:"linkingId,omitempty"`
	AnonymousAddress string `json:"anonymousAddress,omitempty"`
}

// DepositAccountProviderDetailsV3 - details about the provider
type DepositAccountProviderDetailsV3 struct {
	Name             *string `json:"name"`
	ID               *string `json:"id"`
	AnonymousAddress string  `json:"anonymousAddress,omitempty"`
}

// ResponseV3 - wallet creation response
type ResponseV3 struct {
	PaymentID              string                           `json:"paymentId"`
	DepositAccountProvider *DepositAccountProviderDetailsV3 `json:"depositAccountProvider,omitempty"`
	WalletProvider         *ProviderDetailsV3               `json:"walletProvider,omitempty"`
	AltCurrency            string                           `json:"altcurrency"`
	PublicKey              string                           `json:"publicKey"`
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

// ResponseV3ToInfo converts a response v3 to wallet info
func ResponseV3ToInfo(resp ResponseV3) *walletutils.Info {
	alt, _ := altcurrency.FromString(resp.AltCurrency)

	// common to all wallet providers
	info := walletutils.Info{
		ID:          resp.PaymentID,
		AltCurrency: &alt,
		PublicKey:   resp.PublicKey,
	}

	if resp.WalletProvider != nil {
		info.Provider = resp.WalletProvider.Name
		if info.Provider == UpholdProvider {
			// setup the anon card wallet information
			info.ProviderID = resp.WalletProvider.ID
			var providerLinkingID uuid.UUID
			if resp.WalletProvider.LinkingID != "" {
				providerLinkingID = uuid.Must(uuid.FromString(resp.WalletProvider.LinkingID))
			}
			info.ProviderLinkingID = &providerLinkingID

			var anonymousAddress uuid.UUID
			if resp.WalletProvider.AnonymousAddress != "" {
				anonymousAddress = uuid.Must(uuid.FromString(resp.WalletProvider.AnonymousAddress))
			}
			info.AnonymousAddress = &anonymousAddress
		}
	}
	// setup the user deposit account info
	depositAccountProvider := resp.DepositAccountProvider
	if depositAccountProvider != nil {
		info.UserDepositAccountProvider = depositAccountProvider.Name
		providerLinkingID := uuid.Must(uuid.FromString(*depositAccountProvider.ID))
		info.ProviderLinkingID = &providerLinkingID
		anonymousAddress := uuid.Must(uuid.FromString(depositAccountProvider.AnonymousAddress))
		info.AnonymousAddress = &anonymousAddress
	}
	return &info
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

	// common to all wallets
	resp := ResponseV3{
		PaymentID:   info.ID,
		AltCurrency: altCurrency,
		PublicKey:   info.PublicKey,
		WalletProvider: &ProviderDetailsV3{
			Name: info.Provider,
		},
	}

	// setup the wallet provider (anon card uphold)
	if info.Provider == "uphold" {
		// this is a uphold provided wallet (anon card based)
		resp.WalletProvider.ID = info.ProviderID
		resp.WalletProvider.AnonymousAddress = anonymousAddress
		resp.WalletProvider.LinkingID = linkingID
	}

	// now setup user deposit account
	if info.UserDepositAccountProvider != nil {
		// this brave wallet has a linked deposit account
		resp.DepositAccountProvider = &DepositAccountProviderDetailsV3{
			Name:             info.UserDepositAccountProvider,
			ID:               &linkingID,
			AnonymousAddress: anonymousAddress,
		}
	}

	return resp
}

// BalanceResponseV3 - wallet creation response
type BalanceResponseV3 struct {
	Total       float64 `json:"total,omitempty"`
	Spendable   float64 `json:"spendable,omitempty"`
	Confirmed   float64 `json:"confirmed,omitempty"`
	Unconfirmed float64 `json:"unconfirmed,omitempty"`
}

func balanceToResponseV3(b walletutils.Balance) BalanceResponseV3 {
	// convert to double, don't care about rounding
	total, _ := b.TotalProbi.Float64()
	spendable, _ := b.SpendableProbi.Float64()
	confirmed, _ := b.ConfirmedProbi.Float64()
	unconfirmed, _ := b.UnconfirmedProbi.Float64()

	return BalanceResponseV3{
		Total:       total,
		Spendable:   spendable,
		Confirmed:   confirmed,
		Unconfirmed: unconfirmed,
	}
}
