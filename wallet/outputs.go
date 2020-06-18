package wallet

import uuid "github.com/satori/go.uuid"

const (
	InvalidCurrency = "invalid"
	BATCurrency     = "BAT"
	BTCCurrency     = "BTC"
	ETHCurrency     = "ETH"
	LTCCurrency     = "LTC"
)

// ProviderDetails - details about the provider
type ProviderDetails struct {
	Name      string `json:"provider"`
	ID        string `json:"providerId"`
	LinkingID string `json:"providerLinkingId"`
}

// UpholdCreationResponse - wallet creation response
type UpholdCreationResponse struct {
	PaymentID   uuid.UUID       `json:"paymentId"`
	Provider    ProviderDetails `json:"provider"`
	AltCurrency string          `json:"altcurrency"`
	PublicKey   string          `json:"publicKey"`
}
