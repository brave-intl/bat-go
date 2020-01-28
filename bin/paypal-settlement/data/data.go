package data

import (
	"crypto/sha256"
	"strings"
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
	Date     string
	Rate     decimal.Decimal
	Out      string
}

// CheckDate parses a date from the input string to validate it
func (args Args) CheckDate() (time.Time, error) {
	return time.Parse(time.RFC3339, args.Date+"T00:00:00.0Z")
}

// PayoutTransaction holds all information relevant to a transaction (in BAT)
type PayoutTransaction struct {
	TransactionID      uuid.UUID               `json:"transactionId"`
	PotentialPaymentID uuid.UUID               `json:"potentialPaymentId"`
	Provider           string                  `json:"walletProvider"`
	ProviderID         string                  `json:"walletProviderId"`
	Channel            string                  `json:"publisher"`
	AltCurrency        altcurrency.AltCurrency `json:"altcurrency"`
	Probi              decimal.Decimal         `json:"probi"`
	Address            *uuid.UUID              `json:"address"`
	Authority          string                  `json:"authority"`
	Fees               decimal.Decimal         `json:"fees"`
	Publisher          string                  `json:"owner"`
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

// RefIDKey is used to generate a hash
func (pm *PaypalMetadata) RefIDKey() string {
	return pm.Prefix + pm.Publisher + pm.Channel
}

// GenerateRefID converts a hex to base62
func (pm *PaypalMetadata) GenerateRefID() string {
	key := pm.RefIDKey() // nothing more needed for now
	bytes := sha256.Sum256([]byte(key))
	refID := base58.Encode(bytes[:], base58.BitcoinAlphabet)
	refID = refID[:30]
	pm.RefID = refID
	return refID
}

// PaypalMetadata holds metadata to create a row for paypal
type PaypalMetadata struct {
	Prefix        string
	Section       string
	PayerID       string
	Channel       string
	Publisher     string
	Amount        decimal.Decimal
	Currency      string
	RefID         string
	Note          []string
	NoteDelimiter string
}

// NewPaypalMetadata creates a new paypal metadata object
func NewPaypalMetadata(pm PaypalMetadata) PaypalMetadata {
	delimiter := "\n"
	if len(pm.NoteDelimiter) > 0 {
		delimiter = pm.NoteDelimiter
	}
	return PaypalMetadata{
		Prefix:        pm.Prefix,
		Section:       pm.Section,
		PayerID:       pm.PayerID,
		Publisher:     pm.Publisher,
		Channel:       pm.Channel,
		Amount:        pm.Amount,
		Currency:      pm.Currency,
		RefID:         pm.GenerateRefID(),
		Note:          pm.Note,
		NoteDelimiter: delimiter,
	}
}

// // ToCSVRow turns a paypal metadata into a list of strings ready to be consumed by a CSV generator
// func (pm *PaypalMetadata) ToCSVRow() []string {
// 	return []string{
// 		pm.Section,
// 		pm.PayerID,
// 		pm.Amount.String(),
// 		pm.Currency,
// 		pm.RefID,
// 		strings.Join(pm.Note, pm.NoteDelimiter),
// 	}
// }

// ToCSVRow turns a paypal metadata into a list of strings ready to be consumed by a CSV generator
func (pm *PaypalMetadata) ToCSVRow() []string {
	return []string{
		pm.PayerID,
		pm.Amount.String(),
		pm.Currency,
		pm.RefID,
		"Payout for\n" + strings.Join(pm.Note, pm.NoteDelimiter),
		"PayPal",
	}
}
