package transform

import (
	"crypto/sha256"
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

	"github.com/brave-intl/bat-go/bin/vault-paypal-settlement/data"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/shopspring/decimal"
)

func Input(args data.Args) (err error) {
	fmt.Println("RUNNING: transform")
	if args.In == "" {
		return errors.New("the 'in' flag must be set")
	}
	if args.Currency == "" {
		return errors.New("the 'currency' flag must be set")
	}

	bytes, err := ioutil.ReadFile(args.In)
	if err != nil {
		return err
	}
	var batPayouts []data.PayoutTransaction
	err = json.Unmarshal(bytes, &batPayouts)
	if err != nil {
		return err
	}

	rateData, err := GetRate(args)
	if err != nil {
		return err
	}
	rate := rateData.Payload[args.Currency]

	file, err := os.Create("result.csv")
	if err != nil {
		return err
	}
	defer file.Close()
	writer := csv.NewWriter(file)
	defer writer.Flush()

	oneHundred := decimal.NewFromFloat(100)
	rows := make([][]string, 0)
	total := decimal.NewFromFloat(0)
	idsInBatch := map[string]bool{}
	for _, batPayout := range batPayouts {
		amount := altcurrency.BAT.FromProbi(rate.Mul(batPayout.Probi)).
			Mul(oneHundred).
			Floor().
			Div(oneHundred)

		// needs to be shortened (0-9,a-Z) <=30 chars
		key := (strings.Join(strings.Split(batPayout.Address.String(), "-"), "") + batPayout.Publisher)
		bytes := sha256.Sum256([]byte(key))
		refID := data.Base62(bytes[:])
		note := "Thanks!"
		row := []string{
			"PAYOUT",
			batPayout.ProviderID,
			amount.String(),
			args.Currency,
			refID,
			note,
		}
		total = total.Add(amount)
		if idsInBatch[refID] {
			fmt.Println("ref id:\t", refID)
			fmt.Println("payout address:\t", batPayout.Address.String())
			fmt.Println("key:", key)
			fmt.Println("bytes length:", strconv.Itoa(len(bytes)))
			return errors.New("id already seen in batch")
		}
		idsInBatch[refID] = true
		rows = append(rows, row)
	}

	err = writeCSVRows(writer, [][]string{
		{
			"PAYOUT_SUMMARY",
			total.String(),
			args.Currency,
			strconv.Itoa(len(rows)),
			"Brave Publishers Payout",
			"Payout for",
		},
	})
	if err != nil {
		return err
	}
	err = writeCSVRows(writer, rows)
	if err != nil {
		return err
	}
	return nil
}

func writeCSVRows(writer *csv.Writer, rows [][]string) error {
	for _, row := range rows {
		err := writer.Write(row)
		if err != nil {
			return err
		}
	}
	return nil
}

// GetRate gets the rate of a currency to BAT
func GetRate(args data.Args) (*data.RateResponse, error) {
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
