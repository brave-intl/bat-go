package paypal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/brave-intl/bat-go/settlement"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/clients/ratios"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

// CreateSettlementFile starts the transform process
func CreateSettlementFile(args TransformArgs) (err error) {
	fmt.Println("RUNNING: transform")
	if args.In == "" {
		return errors.New("the 'in' flag must be set")
	}
	if args.Currency == "" {
		return errors.New("the 'currency' flag must be set")
	}

	payouts, err := ReadFiles(args.In)
	if err != nil {
		return err
	}

	rate, err := GetRate(args.Currency, args.Rate, args.Auth)
	if err != nil {
		return err
	}
	args.Rate = rate

	txs, err := GenerateTransactions(args.Currency, args.Rate, payouts)
	if err != nil {
		return err
	}

	err = WriteTransactions(args.Out+".json", txs)
	if err != nil {
		return err
	}

	metadata, err := ValidatePayouts(args.Currency, txs)
	if err != nil {
		return err
	}

	err = WriteTransformedCSV(args.Currency, args.Rate, args.Out, metadata)
	if err != nil {
		return err
	}
	return nil
}

// GenerateTransactions creates tx structs
func GenerateTransactions(currency string, rate decimal.Decimal, payouts *[]settlement.Transaction) (*[]settlement.Transaction, error) {
	txs := make([]settlement.Transaction, 0)
	transactionID := uuid.NewV4()
	bat := altcurrency.BAT
	for _, tx := range *payouts {
		if tx.WalletProvider != "paypal" {
			continue
		}
		publisher := tx.Publisher
		amount := fromProbi(tx.Probi, rate, currency)
		txs = append(txs, settlement.Transaction{
			ID:               transactionID.String(),
			AltCurrency:      &bat,
			Amount:           amount,
			Authority:        tx.Authority,
			Publisher:        publisher,
			Channel:          tx.Channel,
			Destination:      tx.Destination,
			Probi:            tx.Probi,
			Type:             tx.Type,
			WalletProviderID: tx.WalletProviderID,
			WalletProvider:   tx.WalletProvider,
			Status:           "pending",
			BATPlatformFee:   tx.BATPlatformFee,       // 5%
			ExchangeFee:      decimal.NewFromFloat(0), // should probably be computed
			TransferFee:      decimal.NewFromFloat(0), // should probably be computed
			Currency:         currency,
			ProviderID:       uuid.NewV4().String(),
		})
	}
	return &txs, nil
}

// WriteTransactions outputs json
func WriteTransactions(output string, metadata *[]settlement.Transaction) error {
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(output, data, 0600)
}

// ValidatePayouts validates the payout objects and creates metadata to represent rows
func ValidatePayouts(currency string, batPayouts *[]settlement.Transaction) (*[]Metadata, error) {
	executedAt := time.Now()
	rows := make([]Metadata, 0)
	type placement struct {
		Valid bool
		Index int
	}
	refIDsInBatch := map[string]placement{}
	publishersInBatch := map[string]placement{}

	for i, batPayout := range *batPayouts {
		row := FillMetadataDefaults(Metadata{
			ExecutedAt: executedAt,
			Currency:   currency,
			PayerID:    batPayout.WalletProviderID,
			Publisher:  batPayout.Publisher,
			Channel:    batPayout.Channel,
			Probi:      batPayout.Probi,
		})

		// ref id cannot be same. otherwise we're payout out to same channel twice
		known := refIDsInBatch[row.RefID]
		if known.Valid {
			fmt.Println("ref id:\t", row.RefID)
			fmt.Println("channel:\t", batPayout.Channel)
			fmt.Println("publisher:\t", batPayout.Publisher)
			fmt.Println("hashed key:\t", row.RefIDKey(batPayout.Channel))
			fmt.Println("indices:\t", known.Index, i)
			fmt.Printf("%#v", rows[known.Index])
			return nil, errors.New("id already seen in batch")
		}

		note := Note{batPayout.Channel, batPayout.Probi}
		publisherInBatch := publishersInBatch[batPayout.Publisher]
		if publisherInBatch.Valid {
			cachedRow := rows[publisherInBatch.Index]
			if cachedRow.Channel == batPayout.Channel {
				return nil, errors.New("duplicate payout for: " + batPayout.Channel)
			}
			cachedRow.AddOutput(batPayout.Probi, note)
			refIDsInBatch[row.RefID] = publisherInBatch
			// set back on rows, because not using pointers
			rows[publisherInBatch.Index] = cachedRow
		} else {
			placing := placement{true, len(rows)}
			refIDsInBatch[row.RefID] = placing
			publishersInBatch[batPayout.Publisher] = placing
			row.AddNote(note)
			rows = append(rows, row)
		}
	}
	return &rows, nil
}

// WriteTransformedCSV opens and writes a csv
func WriteTransformedCSV(currency string, rate decimal.Decimal, out string, metadata *[]Metadata) error {
	rows := make([][]string, 0)
	total := decimal.NewFromFloat(0)
	for _, row := range *metadata {
		rows = append(rows, row.ToCSVRow(rate))
		total = total.Add(row.Amount(rate))
	}
	if len(rows) > 5000 {
		return errors.New("a payout cannot be larger than 5000 lines items long")
	}
	fmt.Println("payouts", len(rows))
	fmt.Println("total", total.String(), currency)
	return WriteCSV(out+".csv", append([][]string{
		{
			"Email/Phone",
			"Amount",
			"Currency code",
			"Reference ID",
			"Note to recipient",
			"Recipient wallet",
		},
	}, rows...))
}

// GetRate figures out which rate to use
func GetRate(currency string, rate decimal.Decimal, auth string) (decimal.Decimal, error) {
	if rate.Equal(decimal.NewFromFloat(0)) {
		client, err := ratios.New()
		if err != nil {
			return rate, err
		}
		ctx := context.Background()
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
