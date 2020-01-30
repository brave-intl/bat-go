package paypal

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/settlement"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

// CreateSettlementFile starts the transform process
func CreateSettlementFile(args Args) (err error) {
	fmt.Println("RUNNING: transform")
	if args.In == "" {
		return errors.New("the 'in' flag must be set")
	}
	if args.Currency == "" {
		return errors.New("the 'currency' flag must be set")
	}
	// check that date is fine
	_, err = args.CheckDate()
	if err != nil {
		return err
	}

	payouts, err := ReadFiles(args.In)
	if err != nil {
		return err
	}

	rate, err := GetRate(args)
	if err != nil {
		return err
	}
	args.Rate = rate

	txs, err := CreateEyeshadeTransactions(args, payouts)
	if err != nil {
		return err
	}

	err = WriteEyeshadeTransactions(args.Out, txs)
	if err != nil {
		return err
	}

	metadata, err := ValidatePayouts(args, *payouts)
	if err != nil {
		return err
	}

	err = WriteTransformedCSV(args, *metadata)
	if err != nil {
		return err
	}
	return nil
}

// CreateEyeshadeTransactions creates tx structs
func CreateEyeshadeTransactions(args Args, payouts *[]settlement.Transaction) (*[]settlement.Transaction, error) {
	txs := make([]settlement.Transaction, 0)
	transactionID := uuid.NewV4()
	bat := altcurrency.BAT
	for _, tx := range *payouts {
		publisher := tx.Publisher
		providerID := tx.ProviderID
		if providerID == "" {
			providerID = publisher
		}
		amount := fromProbi(tx.Probi, args.Rate, args.Currency)
		txs = append(txs, settlement.Transaction{
			ID:             transactionID.String(),
			AltCurrency:    &bat,
			Amount:         amount,
			Authority:      tx.Authority,
			Publisher:      publisher,
			Channel:        tx.Channel,
			Destination:    tx.Destination,
			Probi:          tx.Probi,
			Type:           tx.Type,
			ProviderID:     providerID,
			Provider:       tx.Provider,
			Status:         "pending",
			BATPlatformFee: tx.BATPlatformFee,       // 5%
			ExchangeFee:    decimal.NewFromFloat(0), // should probably be computed
			TransferFee:    decimal.NewFromFloat(0), // should probably be computed
			Currency:       args.Currency,
			Hash:           uuid.NewV4().String(),
		})
	}
	return &txs, nil
}

// WriteEyeshadeTransactions outputs json
func WriteEyeshadeTransactions(output string, metadata *[]settlement.Transaction) error {
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(output+".json", data, 0400)
}

// ReadFiles reads a series of files
func ReadFiles(input string) (*[]settlement.Transaction, error) {
	var allPayouts []settlement.Transaction
	files := strings.Split(input, ",")
	for _, file := range files {
		var batPayouts []settlement.Transaction
		bytes, err := ioutil.ReadFile(file)
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(bytes, &batPayouts)
		if err != nil {
			return nil, err
		}
		// TEMPORARY HACK. NEED UPDATED DATA STRUCTURE
		for i, payout := range batPayouts {
			payout.Provider = "paypal"
			batPayouts[i] = payout
		}
		allPayouts = append(allPayouts, batPayouts...)
	}
	return &allPayouts, nil
}

// ValidatePayouts validates the payout objects and creates metadata to represent rows
func ValidatePayouts(args Args, batPayouts []settlement.Transaction) (*[]Metadata, error) {
	// scale := decimal.NewFromFloat(supportedCurrencies[args.Currency])
	// factor := decimal.NewFromFloat(10).Pow(scale)
	executedAt := time.Now()
	rows := make([]Metadata, 0)
	type placement struct {
		Valid bool
		Index int
	}
	refIDsInBatch := map[string]placement{}
	publishersInBatch := map[string]placement{}

	for i, batPayout := range batPayouts {
		if batPayout.Provider != "paypal" {
			continue
		}

		publisher := batPayout.Publisher
		payerID := batPayout.ProviderID
		if payerID == "" {
			payerID = publisher
		}
		row := NewMetadata(Metadata{
			ExecutedAt: executedAt,
			Prefix:     args.Date,
			Section:    "PAYOUT",
			PayerID:    payerID,
			Publisher:  publisher,
			Channel:    batPayout.Channel,
			Probi:      batPayout.Probi,
			Currency:   args.Currency,
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
		publisherInBatch := publishersInBatch[publisher]
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
			publishersInBatch[publisher] = placing
			row.AddNote(note)
			rows = append(rows, row)
		}
	}
	return &rows, nil
}

// WriteTransformedCSV opens and writes a csv
func WriteTransformedCSV(args Args, metadata []Metadata) error {
	rows := make([][]string, 0)
	total := decimal.NewFromFloat(0)
	for _, row := range metadata {
		rows = append(rows, row.ToCSVRow(args.Rate))
		total = total.Add(row.Amount(args.Rate))
	}
	// if len(rows) > 5000 {
	// 	return errors.New("a payout cannot be larger than 5000 lines items long")
	// }
	fmt.Println("payouts", len(rows))
	return WriteCSV(args.Out+".csv", append([][]string{
		{
			"Email/Phone",
			"Amount",
			"Currency code",
			"Reference ID",
			"Note to recipient",
			"Recipient wallet",
		},
	}, rows...))
	// WriteCSV(args.Out, append([][]string{
	// 	{
	// 		"PAYOUT_SUMMARY",
	// 		total.String(),
	// 		args.Currency,
	// 		strconv.Itoa(len(rows)),
	// 		"Brave Publishers Payout",
	// 		"Payout for",
	// 	},
	// }, rows...))
}

// WriteCSV writes out a csv
func WriteCSV(out string, rows [][]string) error {
	file, err := os.Create(out)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	writer := csv.NewWriter(file)
	defer writer.Flush()
	return WriteCSVRows(writer, rows)
}

// WriteCSVRows writes rows into a csv writer
func WriteCSVRows(writer *csv.Writer, rows [][]string) error {
	for _, row := range rows {
		err := writer.Write(row)
		if err != nil {
			return err
		}
	}
	return nil
}

// GetRate figures out which rate to use
func GetRate(args Args) (decimal.Decimal, error) {
	var rate decimal.Decimal
	if args.Rate.Equal(decimal.NewFromFloat(0)) {
		rateData, err := FetchRate(args)
		if err != nil {
			return args.Rate, err
		}
		rate = rateData.Payload[args.Currency]
		if time.Since(rateData.LastUpdated).Minutes() > 5 {
			return args.Rate, errors.New("ratios data is too old. update ratios response before moving forward")
		}
	} else {
		rate = args.Rate
	}
	return rate, nil
}

// FetchRate fetches the rate of a currency to BAT
func FetchRate(args Args) (*RateResponse, error) {
	var body RateResponse
	url := "https://ratios.mercury.basicattentiontoken.org/v1/relative/BAT?currency=" + args.Currency
	bytes, err := Request("GET", url, args)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(bytes, &body)
	if err != nil {
		return nil, err
	}
	return &body, err
}

// Request does a request
func Request(method string, url string, args Args) (body []byte, err error) {
	client := http.Client{
		Timeout: time.Second * 2, // Maximum of 2 secs
	}
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return
	}
	req.Header.Add("Authorization", "Bearer "+args.Auth)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("\n\turl: %s\n\tstatus code: %d", url, resp.StatusCode)
	}
	return body, nil
}
