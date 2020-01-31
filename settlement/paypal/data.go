package paypal

import (
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/brave-intl/bat-go/utils/altcurrency"
	uuid "github.com/satori/go.uuid"
	"github.com/shengdoushi/base58"
	"github.com/shopspring/decimal"
)

var (
	supportedCurrencies = map[string]float64{
		"JPY": 0,
		"USD": 2,
	}
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
func (pm *Metadata) RefIDKey(channel string) string {
	return pm.Prefix + pm.Publisher + channel
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
	ExecutedAt    time.Time
	Rate          decimal.Decimal
	Prefix        string
	Section       string
	PayerID       string
	Channel       string
	Publisher     string
	Probi         decimal.Decimal
	Currency      string
	RefID         string
	Note          []Note
	NoteDelimiter string
}

// EyeshadeTransaction an object to express the flow of funds
type EyeshadeTransaction struct {
	Publisher     string                  `json:"publisher"`
	Owner         string                  `json:"owner"`
	Probi         decimal.Decimal         `json:"probi"`
	Currency      string                  `json:"currency"`
	Hash          string                  `json:"hash"`
	Amount        decimal.Decimal         `json:"amount"`
	ExecutedAt    time.Time               `json:"executedAt"`
	AltCurrency   altcurrency.AltCurrency `json:"altcurrency"`
	Address       uuid.UUID               `json:"address"`
	Fees          decimal.Decimal         `json:"fees"`
	Fee           decimal.Decimal         `json:"fee"`
	Commission    decimal.Decimal         `json:"commission"`
	TransactionID uuid.UUID               `json:"transactionId"`
	Type          string                  `json:"type"`
}

// NewMetadata creates a new paypal metadata object
func NewMetadata(pm Metadata) Metadata {
	delimiter := ","
	if len(pm.NoteDelimiter) > 0 {
		delimiter = pm.NoteDelimiter
	}
	return Metadata{
		ExecutedAt:    pm.ExecutedAt,
		Rate:          pm.Rate,
		Prefix:        pm.Prefix,
		Section:       pm.Section,
		PayerID:       pm.PayerID,
		Publisher:     pm.Publisher,
		Channel:       pm.Channel,
		Probi:         pm.Probi,
		Currency:      pm.Currency,
		RefID:         pm.GenerateRefID(pm.Channel),
		Note:          pm.Note,
		NoteDelimiter: delimiter,
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
	// var notes []string
	probiTotal := decimal.NewFromFloat(0)
	for _, note := range pm.Note {
		probiTotal = probiTotal.Add(note.Probi)
		// notes = append(notes, "("+note.Channel+" -> "+amount.String()+")")
	}
	convertedAmount := fromProbi(probiTotal, rate, pm.Currency)
	batAmount := altcurrency.BAT.FromProbi(probiTotal)
	batFloat := batAmount.String()
	return []string{
		pm.PayerID,
		convertedAmount.String(),
		pm.Currency,
		pm.RefID,
		fmt.Sprintf("You earned %s BAT from %d channel(s).", batFloat, len(pm.Note)),
		// strings.Join(notes, pm.NoteDelimiter),
		"PayPal",
	}
}
