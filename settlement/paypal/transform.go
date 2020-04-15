package paypal

import (
	"context"
	"errors"
	"time"

	"github.com/brave-intl/bat-go/settlement"
	"github.com/brave-intl/bat-go/utils/clients/ratios"
	"github.com/shopspring/decimal"
)

// CalculateTransactionAmounts calculates the amount for each payout given a currency and rate
func CalculateTransactionAmounts(currency string, rate decimal.Decimal, payouts *[]settlement.Transaction) (*[]settlement.Transaction, error) {
	err := settlement.CheckForDuplicates(*payouts)
	if err != nil {
		return nil, err
	}

	txs := make([]settlement.Transaction, 0)
	for _, tx := range *payouts {
		if tx.WalletProvider != "paypal" {
			continue
		}
		tx.Amount = exchangeFromProbi(tx.Probi, rate, currency)
		tx.Currency = currency
		txs = append(txs, tx)
	}
	return &txs, nil
}

// MergeAndTransformPayouts merges payouts to the same destination and transforms to paypal txn metadata
func MergeAndTransformPayouts(batPayouts *[]settlement.Transaction) (*[]Metadata, error) {
	executedAt := time.Now().UTC()
	rows := make([]Metadata, 0)
	destinationToRow := map[string]*Metadata{}

	// FIXME refactor to separate merge and transform
	for _, batPayout := range *batPayouts {
		destination := batPayout.Destination

		var row *Metadata
		var ok bool
		if row, ok = destinationToRow[destination]; !ok {
			row = &Metadata{
				Currency:     batPayout.Currency,
				ExecutedAt:   executedAt,
				PayerID:      batPayout.Destination,
				SettlementID: batPayout.SettlementID,
			}
			err := row.GenerateRefID()
			if err != nil {
				return nil, err
			}
			destinationToRow[destination] = row
		}

		err := row.AddTransaction(batPayout)
		if err != nil {
			return nil, err
		}
	}
	for _, row := range destinationToRow {
		rows = append(rows, *row)
	}
	return &rows, nil
}

// GetRate figures out which rate to use
func GetRate(ctx context.Context, currency string, rate decimal.Decimal) (decimal.Decimal, error) {
	if rate.Equal(decimal.NewFromFloat(0)) {
		client, err := ratios.NewWithContext(ctx)
		if err != nil {
			return rate, err
		}
		rateData, err := client.FetchRate(ctx, "BAT", currency)
		if err != nil {
			return rate, err
		}
		if rateData == nil {
			return rate, errors.New("ratio not found")
		}
		rate = rateData.Payload[currency]
		if time.Since(rateData.LastUpdated).Minutes() > 5 {
			return rate, errors.New("ratios data is too old. update ratios response before moving forward")
		}
	}
	return rate, nil
}
