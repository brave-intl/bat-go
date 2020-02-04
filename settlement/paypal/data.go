package paypal

import (
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/shengdoushi/base58"
	"github.com/shopspring/decimal"
)

var (
	supportedCurrencies = map[string]float64{
		"JPY": 0,
		"USD": 2,
	}
	currencySymbols = map[string]string{
		"USD": "$",
		"JPY": "¥",
	}
)

// TransformArgs are the args required for the transform command
type TransformArgs struct {
	In       string
	Currency string
	Auth     string
	Rate     decimal.Decimal
	Out      string
}

// CompleteArgs are the args required for the complete command
type CompleteArgs struct {
	In  string
	Out string
}

// RateResponse is the response received from ratios
type RateResponse struct {
	LastUpdated time.Time                  `json:"lastUpdated"`
	Payload     map[string]decimal.Decimal `json:"payload"`
}

// RefIDKey is used to generate a hash
func (pm *Metadata) RefIDKey(channel string) string {
	return pm.Publisher + channel
}

// GenerateRefID converts a hex to base62
func (pm *Metadata) GenerateRefID(channel string) string {
	key := pm.RefIDKey(channel) // nothing more needed for now
	bytes := sha256.Sum256([]byte(key))
	refID := base58.Encode(bytes[:], base58.BitcoinAlphabet)
	refID = refID[:30]
	pm.RefID = refID
	return refID
}

// Note a note to be transformed into personal email message
type Note struct {
	Channel string
	Probi   decimal.Decimal
}

// Metadata holds metadata to create a row for paypal
type Metadata struct {
	ExecutedAt time.Time
	Rate       decimal.Decimal
	Section    string
	PayerID    string
	Channel    string
	Publisher  string
	Probi      decimal.Decimal
	Currency   string
	RefID      string
	Note       []Note
}

// FillMetadataDefaults backfills the defaults in a metadata object
func FillMetadataDefaults(pm Metadata) Metadata {
	return Metadata{
		Section:    "PAYOUT",
		ExecutedAt: pm.ExecutedAt,
		Rate:       pm.Rate,
		PayerID:    pm.PayerID,
		Publisher:  pm.Publisher,
		Channel:    pm.Channel,
		Probi:      pm.Probi,
		Currency:   pm.Currency,
		RefID:      pm.GenerateRefID(pm.Channel),
		Note:       pm.Note,
	}
}

// AddOutput adds a transaction output to the row
func (pm *Metadata) AddOutput(probi decimal.Decimal, note Note) {
	pm.Probi = pm.Probi.Add(probi)
	pm.Note = append(pm.Note, note)
}

// AddNote adds a note
func (pm *Metadata) AddNote(note Note) {
	pm.Note = append(pm.Note, note)
}

func fromProbi(probi decimal.Decimal, rate decimal.Decimal, currency string) decimal.Decimal {
	scale := decimal.NewFromFloat(supportedCurrencies[currency])
	factor := decimal.NewFromFloat(10).Pow(scale)
	return altcurrency.BAT.
		FromProbi(rate.Mul(probi)).
		Mul(factor).
		Floor().
		Div(factor)
}

// Amount adds all probi up and pays that amount out
func (pm *Metadata) Amount(rate decimal.Decimal) decimal.Decimal {
	total := decimal.NewFromFloat(0)
	for _, note := range pm.Note {
		total = total.Add(note.Probi)
	}
	return fromProbi(total, rate, pm.Currency)
}

// ToCSVRow turns a paypal metadata into a list of strings ready to be consumed by a CSV generator
func (pm *Metadata) ToCSVRow(rate decimal.Decimal) []string {
	probiTotal := decimal.NewFromFloat(0)
	for _, note := range pm.Note {
		probiTotal = probiTotal.Add(note.Probi)
	}
	convertedAmount := fromProbi(probiTotal, rate, pm.Currency)
	batAmount := altcurrency.BAT.FromProbi(probiTotal)
	batFloat := batAmount.String()
	convertedAmountWithUnit := currencySymbols[pm.Currency] + convertedAmount.String()
	batFloatWithUnit := batFloat + " BAT"
	if pm.Currency == "JPY" {
		batFloatWithUnit = batFloatWithUnit + " Points"
	}
	return []string{
		pm.PayerID,
		convertedAmount.String(),
		pm.Currency,
		pm.RefID,
		fmt.Sprintf("You earned %s, as %s from %d channel(s).", batFloatWithUnit, convertedAmountWithUnit, len(pm.Note)),
		"PayPal",
	}
}
