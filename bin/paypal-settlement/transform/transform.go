package transform

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/bin/paypal-settlement/data"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/shopspring/decimal"
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
func ReadFiles(input string) (*[]data.PayoutTransaction, error) {
	var allPayouts []data.PayoutTransaction
	files := strings.Split(input, ",")
	for _, file := range files {
		var batPayouts []data.PayoutTransaction
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
func ValidatePayouts(args data.Args, rate decimal.Decimal, batPayouts []data.PayoutTransaction) (*[]data.PaypalMetadata, error) {
	oneHundred := decimal.NewFromFloat(100)
	rows := make([]data.PaypalMetadata, 0)
	type placement struct {
		Valid bool
		Index int
	}
	refIDsInBatch := map[string]placement{}
	ownersInBatch := map[string]placement{}

	for i, batPayout := range batPayouts {
		amount := altcurrency.BAT.FromProbi(rate.Mul(batPayout.Probi)).
			Mul(oneHundred).
			Floor().
			Div(oneHundred)

		// needs to be shortened (0-9,a-Z) <=30 chars
		owner := batPayout.Owner
		amountString := amount.String()
		note := "(" + batPayout.Publisher + " -> " + amountString + ")"
		row := data.NewPaypalMetadata(data.PaypalMetadata{
			Prefix:    args.Date,
			Section:   "PAYOUT",
			PayerID:   batPayout.ProviderID,
			Owner:     owner,
			Publisher: batPayout.Publisher,
			Amount:    amount,
			Currency:  args.Currency,
			Note:      []string{note},
		})
		refID := row.GenerateRefID()

		// ref id cannot be same. otherwise we're payout out to same channel twice
		known := refIDsInBatch[refID]
		if known.Valid {
			fmt.Println("ref id:\t", refID)
			fmt.Println("payout address:\t", batPayout.Address.String())
			fmt.Println("publisher:", batPayout.Publisher)
			fmt.Println("owner:", batPayout.Owner)
			fmt.Println("hashed key:\t", row.RefIDKey())
			fmt.Println("indices:", known.Index, i)
			fmt.Printf("%#v", rows[known.Index])
			return nil, errors.New("id already seen in batch")
		}

		ownerInBatch := ownersInBatch[owner]
		if ownerInBatch.Valid {
			row := rows[ownerInBatch.Index]
			if row.Publisher == batPayout.Publisher {
				return nil, errors.New("duplicate payout for:" + batPayout.Publisher)
			}
			row.Amount = row.Amount.Add(amount)
			row.Note = append(row.Note, note)
			rows[ownerInBatch.Index] = row
		} else {
			placing := placement{true, len(rows)}
			refIDsInBatch[refID] = placing
			ownersInBatch[owner] = placing
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
	return WriteCSV(args.Out, append([][]string{
		{
			"PAYOUT_SUMMARY",
			total.String(),
			args.Currency,
			strconv.Itoa(len(rows)),
			"Brave Publishers Payout",
			"Payout for",
		},
	}, rows...))
}

// WriteCSV writes out a csv
func WriteCSV(out string, rows [][]string) error {
	file, err := os.Create(out)
	if err != nil {
		return err
	}
	defer file.Close()
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
	defer resp.Body.Close()
	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("\n\turl: %s\n\tstatus code: %d", url, resp.StatusCode)
	}
	return body, nil
}
