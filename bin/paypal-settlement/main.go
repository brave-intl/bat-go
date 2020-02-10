package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/brave-intl/bat-go/settlement"
	"github.com/brave-intl/bat-go/settlement/paypal"
	"github.com/brave-intl/bat-go/utils/closers"
	"github.com/gocarina/gocsv"
	"github.com/shopspring/decimal"
)

var (
	input    = flag.String("in", "", "the file or comma delimited list of files that should be utilized")
	currency = flag.String("currency", "", "a currency must be set")
	txnID    = flag.String("txnID", "", "the completed mass pay transaction id")
	auth     = os.Getenv("RATE_AUTH")
	rate     = flag.Float64("rate", 0, "the rate to compute the currency conversion")
	out      = flag.String("out", "./paypal-settlement", "the location of the file")
)

// TransformArgs are the args required for the transform command
type TransformArgs struct {
	In       string
	Currency string
	Auth     string
	Rate     decimal.Decimal
	Out      string
}

// WriteTransactions writes settlement transactions to a json file
func WriteTransactions(outPath string, metadata *[]settlement.Transaction) error {
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(outPath, data, 0600)
}

// WriteMassPayCSV writes a csv for using with Paypal web mass payments
func WriteMassPayCSV(outPath string, metadata *[]paypal.Metadata) error {
	rows := []*paypal.MassPayRow{}
	total := decimal.NewFromFloat(0)
	currency := ""
	for _, entry := range *metadata {
		row := entry.ToMassPayCSVRow()
		total = total.Add(row.Amount)
		currency = row.Currency
		rows = append(rows, row)
	}
	if len(rows) > 5000 {
		return errors.New("a payout cannot be larger than 5000 lines items long")
	}
	fmt.Println("payouts", len(rows))
	fmt.Println("total", total.String(), currency)

	data, err := gocsv.MarshalString(&rows)
	if err != nil {
		return err
	}

	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer closers.Panic(f)
	_, err = f.WriteString(data)
	if err != nil {
		return err
	}
	return nil
}

// TransformForMassPay starts the process to transform a settlement into a mass pay csv
func TransformForMassPay(args TransformArgs) (err error) {
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

	rate, err := paypal.GetRate(args.Currency, args.Rate, args.Auth)
	if err != nil {
		return err
	}
	args.Rate = rate

	txs, err := paypal.CalculateTransactionAmounts(args.Currency, args.Rate, payouts)
	if err != nil {
		return err
	}

	err = WriteTransactions(args.Out+".json", txs)
	if err != nil {
		return err
	}

	metadata, err := paypal.MergeAndTransformPayouts(txs)
	if err != nil {
		return err
	}

	err = WriteMassPayCSV(args.Out+".csv", metadata)
	if err != nil {
		return err
	}
	return nil
}

// CompleteSettlement marks the settlement file as complete
func CompleteSettlement(inPath string, outPath string, txnID string) error {
	fmt.Println("RUNNING: complete")
	if inPath == "" {
		return errors.New("the 'in' flag must be set")
	}
	if txnID == "" {
		return errors.New("the 'txnID' flag must be set")
	}
	if outPath == "./paypal-settlement" {
		// use a file with extension if none is passed
		outPath = "./paypal-settlement-complete.json"
	}
	payouts, err := ReadFiles(inPath)
	if err != nil {
		return err
	}
	for i, payout := range *payouts {
		if payout.WalletProvider != "paypal" {
			return errors.New("Error, non-paypal payment included.\nThis command should be called only on the filtered paypal-settlement.json")
		}
		payout.Status = "complete"
		payout.ProviderID = txnID
		(*payouts)[i] = payout
	}
	err = WriteTransactions(outPath, payouts)
	if err != nil {
		return err
	}
	return nil
}

func main() {
	var err error
	flag.Parse()
	command := flag.Arg(0)
	switch command {
	case "transform":
		err = TransformForMassPay(TransformArgs{
			In:       *input,
			Currency: *currency,
			Auth:     auth,
			Rate:     decimal.NewFromFloat(*rate),
			Out:      *out,
		})
	case "complete":
		err = CompleteSettlement(*input, *out, *txnID)
	case "upload":
		// upload()
	case "verify":
		// verify()
	default:
		err = errors.New("a command must be passed (transform, complete)")
	}
	if err != nil {
		flag.Usage()
		fmt.Println("ERROR:", err)
	}
}
