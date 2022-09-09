package paypal

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/brave-intl/bat-go/libs/altcurrency"
	"github.com/brave-intl/bat-go/libs/custodian"
	"github.com/shengdoushi/base58"
	"github.com/shopspring/decimal"
)

var (
	supportedCurrencies = map[string]float64{
		"JPY": 0,
	}
	currencySymbols = map[string]string{
		"JPY": "¥",
	}
)

// GenerateRefID converts a hex to base62
func (pm *Metadata) GenerateRefID() error {
	if len(pm.SettlementID) == 0 || len(pm.PayerID) == 0 {
		return errors.New("must populate SettlementID and PayerID to generate a ref ID")
	}
	key := pm.SettlementID + pm.PayerID
	bytes := sha256.Sum256([]byte(key))
	refID := base58.Encode(bytes[:], base58.BitcoinAlphabet)
	refID = refID[:30]
	pm.RefID = refID
	return nil
}

// FIXME make this a generic merged payment

// Metadata holds metadata to create a row for paypal
type Metadata struct {
	Amount       decimal.Decimal
	BATAmount    decimal.Decimal
	Currency     string
	ExecutedAt   time.Time
	Transactions []custodian.Transaction
	ChannelCount int
	PayerID      string
	RefID        string
	SettlementID string
}

// AddTransaction to the aggregate payment
func (pm *Metadata) AddTransaction(transaction custodian.Transaction) error {
	if pm.Currency != transaction.Currency {
		return errors.New("currency in aggregate payment did not match existing currency")
	}
	pm.ChannelCount++
	pm.BATAmount = pm.BATAmount.Add(altcurrency.BAT.FromProbi(transaction.Probi))
	pm.Amount = pm.Amount.Add(transaction.Amount)
	pm.Transactions = append(pm.Transactions, transaction)
	return nil
}

func exchangeFromProbi(probi decimal.Decimal, rate decimal.Decimal, currency string) decimal.Decimal {
	scale := decimal.NewFromFloat(supportedCurrencies[currency])
	factor := decimal.NewFromFloat(10).Pow(scale)
	return altcurrency.BAT.
		FromProbi(rate.Mul(probi)).
		Mul(factor).
		Floor().
		Div(factor)
}

// MassPayRow is the structure of a row used for paypal web mass pay
type MassPayRow struct {
	PayerID         string          `csv:"Email/Phone"`
	Amount          decimal.Decimal `csv:"Amount"`
	Currency        string          `csv:"Currency code"`
	ID              string          `csv:"Reference ID"`
	Note            string          `csv:"Note to recipient"`
	DestinationType string          `csv:"Recipient wallet"`
}

// ToMassPayCSVRow turns a paypal metadata into a MassPayRow
func (pm *Metadata) ToMassPayCSVRow() *MassPayRow {
	return &MassPayRow{
		PayerID:         pm.PayerID,
		Amount:          pm.Amount,
		Currency:        pm.Currency,
		ID:              pm.RefID,
		Note:            fmt.Sprintf("You earned %s BAT Points, as %s %s from %d channel(s).", pm.BATAmount.String(), pm.Amount.String(), currencySymbols[pm.Currency], pm.ChannelCount),
		DestinationType: "PayPal",
	}
}
