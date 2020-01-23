package data

import (
	"time"

	"github.com/brave-intl/bat-go/utils/altcurrency"
	uuid "github.com/satori/go.uuid"
	"github.com/shengdoushi/base58"
	"github.com/shopspring/decimal"
)

// Args defines all inputs from cli
type Args struct {
	In       string
	Currency string
	Auth     string
}

// PayoutTransaction holds all information relevant to a transaction (in BAT)
type PayoutTransaction struct {
	TransactionID      uuid.UUID               `json:"transactionId"`
	PotentialPaymentID uuid.UUID               `json:"potentialPaymentId"`
	ProviderID         string                  `json:"upholdId"`
	Publisher          string                  `json:"publisher"`
	AltCurrency        altcurrency.AltCurrency `json:"altcurrency"`
	Probi              decimal.Decimal         `json:"probi"`
	Address            *uuid.UUID              `json:"address"`
	Authority          string                  `json:"authority"`
	Fees               decimal.Decimal         `json:"fees"`
	Owner              string                  `json:"owner"`
	Type               string                  `json:"type"`
	// HContributionsVerdict *bool `json:"hContributionsVerdict"`
	// HReferralsVerdict     *bool `json:"hReferralsVerdict"`
	// HOwnerVerdict         *bool `json:"hOwnerVerdict"`
	// HProviderIDVerdict    *bool `json:"hProviderIDVerdict"`
	// HProviderIDNotes      *bool `json:"hProviderIDNotes"`
	// HShadowBanned         *bool `json:"hShadowBanned"`
}

// CurrencyPrices is a hash of all currencies supported
type CurrencyPrices struct {
	JPY decimal.Decimal `json:"JPY"`
}

// RateResponse is the response received from ratios
type RateResponse struct {
	LastUpdated time.Time                  `json:"lastUpdated"`
	Payload     map[string]decimal.Decimal `json:"payload"`
}

// Base62 converts a hex to base62
func Base62(str []byte) string {
	return base58.Encode(str, base58.BitcoinAlphabet)
}
