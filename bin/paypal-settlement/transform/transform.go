package transform

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

	"github.com/brave-intl/bat-go/bin/paypal-settlement/data"
	"github.com/brave-intl/bat-go/settlement"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/shopspring/decimal"
)

var (
	supportedCurrencies = map[string]float64{
		"JPY": 0,
		"USD": 2,
	}
)

// Input starts the transform process
func Input(args data.Args) (err error) {
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

	metadata, err := ValidatePayouts(args, rate, *payouts)
	if err != nil {
		return err
	}

	err = WriteTransformedCSV(args, *metadata)
	if err != nil {
		return err
	}
	return nil
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
		allPayouts = append(allPayouts, batPayouts...)
	}
	return &allPayouts, nil
}

// ValidatePayouts validates the payout objects and creates metadata to represent rows
func ValidatePayouts(args data.Args, rate decimal.Decimal, batPayouts []settlement.Transaction) (*[]data.PaypalMetadata, error) {
	scale := decimal.NewFromFloat(supportedCurrencies[args.Currency])
	factor := decimal.NewFromFloat(10).Pow(scale)
	rows := make([]data.PaypalMetadata, 0)
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
		amount := altcurrency.BAT.FromProbi(rate.Mul(batPayout.Probi)).
			Mul(factor).
			Floor().
			Div(factor)

		// needs to be shortened (0-9,a-Z) <=30 chars
		publisher := batPayout.Publisher
		amountString := amount.String()
		note := "(" + batPayout.Channel + " -> " + amountString + ")"
		row := data.NewPaypalMetadata(data.PaypalMetadata{
			Prefix:    args.Date,
			Section:   "PAYOUT",
			PayerID:   batPayout.ProviderID,
			Publisher: publisher,
			Channel:   batPayout.Channel,
			Amount:    amount,
			Currency:  args.Currency,
			Note: []string{
				note,
			},
		})

		// ref id cannot be same. otherwise we're payout out to same channel twice
		known := refIDsInBatch[row.RefID]
		if known.Valid {
			fmt.Println("ref id:\t", row.RefID)
			fmt.Println("channel:\t", batPayout.Channel)
			fmt.Println("publisher:\t", batPayout.Publisher)
			fmt.Println("hashed key:\t", row.RefIDKey())
			fmt.Println("indices:\t", known.Index, i)
			fmt.Printf("%#v", rows[known.Index])
			return nil, errors.New("id already seen in batch")
		}

		publisherInBatch := publishersInBatch[publisher]
		if publisherInBatch.Valid {
			row := rows[publisherInBatch.Index]
			if row.Channel == batPayout.Channel {
				return nil, errors.New("duplicate payout for: " + batPayout.Channel)
			}
			row.Amount = row.Amount.Add(amount)
			row.Note = append(row.Note, note)
			rows[publisherInBatch.Index] = row
		} else {
			placing := placement{true, len(rows)}
			refIDsInBatch[row.RefID] = placing
			publishersInBatch[publisher] = placing
			rows = append(rows, row)
		}
	}
	return &rows, nil
}

// WriteTransformedCSV opens and writes a csv
func WriteTransformedCSV(args data.Args, metadata []data.PaypalMetadata) error {
	rows := make([][]string, 0)
	total := decimal.NewFromFloat(0)
	for _, row := range metadata {
		rows = append(rows, row.ToCSVRow())
		total = total.Add(row.Amount)
	}
	if len(rows) > 5000 {
		return errors.New("a payout cannot be larger than 5000 lines items long")
	}
	fmt.Println("payouts", len(rows))
	return WriteCSV(args.Out, append([][]string{
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
func GetRate(args data.Args) (decimal.Decimal, error) {
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
func FetchRate(args data.Args) (*data.RateResponse, error) {
	var body data.RateResponse
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
func Request(method string, url string, args data.Args) (body []byte, err error) {
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
