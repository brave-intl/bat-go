package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/brave-intl/bat-go/settlement"
	"github.com/brave-intl/bat-go/settlement/paypal"
	"github.com/brave-intl/bat-go/utils/closers"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/gocarina/gocsv"
	"github.com/shopspring/decimal"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	input             string
	currency          string
	txnID             string
	rate              float64
	out               string
	ratiosAccessToken string
	ratiosServer      string
)

func must(err error) {
	if err != nil {
		log.Printf("failed to initialize: %s\n", err.Error())
		os.Exit(1)
	}
}

func init() {
	// add complete and transform subcommand
	paypalSettlementCmd.AddCommand(completePaypalSettlementCmd)
	paypalSettlementCmd.AddCommand(transformPaypalSettlementCmd)
	paypalSettlementCmd.AddCommand(emailPaypalSettlementCmd)

	// add this command as a settlement subcommand
	settlementCmd.AddCommand(paypalSettlementCmd)

	// setup the flags

	// input (required by all)
	paypalSettlementCmd.PersistentFlags().StringVarP(&input, "input", "i", "",
		"the file or comma delimited list of files that should be utilized")
	must(viper.BindPFlag("input", paypalSettlementCmd.PersistentFlags().Lookup("input")))
	must(viper.BindEnv("input", "INPUT"))
	must(paypalSettlementCmd.MarkPersistentFlagRequired("input"))

	// out (required by all with default)
	paypalSettlementCmd.PersistentFlags().StringVarP(&out, "out", "o", "./paypal-settlement",
		"the location of the file")
	must(viper.BindPFlag("out", paypalSettlementCmd.PersistentFlags().Lookup("out")))
	must(viper.BindEnv("out", "OUT"))

	// currency (required by transform)
	transformPaypalSettlementCmd.PersistentFlags().StringVarP(&currency, "currency", "c", "",
		"a currency must be set")
	must(viper.BindPFlag("currency", transformPaypalSettlementCmd.PersistentFlags().Lookup("currency")))
	must(viper.BindEnv("currency", "CURRENCY"))
	must(transformPaypalSettlementCmd.MarkPersistentFlagRequired("currency"))

	// txnID (required by complete)
	completePaypalSettlementCmd.PersistentFlags().StringVarP(&txnID, "txn-id", "t", "",
		"the completed mass pay transaction id")
	must(viper.BindPFlag("txn-id", paypalSettlementCmd.PersistentFlags().Lookup("txn-id")))
	must(viper.BindEnv("txn-id", "TXN_ID"))
	must(completePaypalSettlementCmd.MarkPersistentFlagRequired("txn-id"))

	// ratios-server
	transformPaypalSettlementCmd.PersistentFlags().StringVarP(&ratiosServer, "ratios-server", "s", "",
		"the ratios server url")
	must(viper.BindPFlag("ratios-server", paypalSettlementCmd.PersistentFlags().Lookup("ratios-server")))
	must(viper.BindEnv("ratios-server", "RATIOS_SERVER"))
	must(transformPaypalSettlementCmd.MarkPersistentFlagRequired("ratios-server"))

	// ratios-access-token
	transformPaypalSettlementCmd.PersistentFlags().StringVarP(&ratiosAccessToken, "ratios-access-token", "a", "",
		"the ratios server url")
	must(viper.BindPFlag("ratios-access-token", paypalSettlementCmd.PersistentFlags().Lookup("ratios-access-token")))
	must(viper.BindEnv("ratios-access-token", "RATIOS_ACCESS_TOKEN"))
	must(transformPaypalSettlementCmd.MarkPersistentFlagRequired("ratios-access-token"))

	// rate
	transformPaypalSettlementCmd.PersistentFlags().Float64VarP(&rate, "rate", "r", 0,
		"the rate to compute the currency conversion")
	must(viper.BindPFlag("rate", transformPaypalSettlementCmd.PersistentFlags().Lookup("rate")))
	must(viper.BindEnv("rate", "RATE"))
}

// EmailTemplate performs template replacement of date fields in emails
func EmailTemplate(inPath string, outPath string) (err error) {
	// read in email template
	data, err := ioutil.ReadFile(inPath)
	if err != nil {
		err = fmt.Errorf("failed to read template: %w", err)
		return
	}
	// perform template rendering to out
	f, err := os.Create(outPath)
	if err != nil {
		err = fmt.Errorf("failed to create output: %w", err)
		return
	}
	defer func() {
		if err = f.Close(); err != nil {
			err = fmt.Errorf("failed to create output: %w", err)
			return
		}
	}()

	var (
		today = time.Now()
		// template will have a "year" and "month" field
		v = struct {
			Month int
			Year  int
		}{
			Month: int(today.Month()),
			Year:  today.Year(),
		}
		t = template.Must(template.New("email").Parse(string(data)))
	)

	if err = t.Execute(f, v); err != nil {
		err = fmt.Errorf("failed to execute template: %w", err)
		return
	}
	return
}

var (
	paypalSettlementCmd = &cobra.Command{
		Use:   "paypal",
		Short: "provides paypal settlement",
	}

	emailPaypalSettlementCmd = &cobra.Command{
		Use:   "email",
		Short: "provides population of a templated email",
		Run: func(cmd *cobra.Command, args []string) {
			if err := EmailTemplate(input, out); err != nil {
				log.Printf("failed to perform email templating: %s\n", err)
				os.Exit(1)
			}
		},
	}

	completePaypalSettlementCmd = &cobra.Command{
		Use:   "complete",
		Short: "provides completion of paypal settlement",
		Run: func(cmd *cobra.Command, args []string) {
			if err := CompleteSettlement(input, out, txnID); err != nil {
				log.Printf("failed to perform complete: %s\n", err)
				os.Exit(1)
			}
		},
	}

	transformPaypalSettlementCmd = &cobra.Command{
		Use:   "transform",
		Short: "provides transform of paypal settlement for mass pay",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// add flag values to our base context that need to be there
			ctx = context.WithValue(ctx, appctx.RatiosServerCTXKey, ratiosServer)
			ctx = context.WithValue(ctx, appctx.RatiosAccessTokenCTXKey, ratiosAccessToken)
		},
		Run: func(cmd *cobra.Command, args []string) {
			if err := TransformForMassPay(TransformArgs{
				In:       input,
				Currency: currency,
				Rate:     decimal.NewFromFloat(rate),
				Out:      out,
			}); err != nil {
				log.Printf("failed to perform transform: %s\n", err)
				os.Exit(1)
			}
		},
	}
)

// CompleteSettlement marks the settlement file as complete
func CompleteSettlement(inPath string, outPath string, txnID string) error {
	fmt.Println("RUNNING: complete")
	if inPath == "" {
		return errors.New("the '-i' or '--input' flag must be set")
	}
	if txnID == "" {
		return errors.New("the '-t' or '--txn-id' flag must be set")
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
		if !payout.Amount.GreaterThan(decimal.Zero) {
			return errors.New("Error, non-zero payment included.\nThis command should be called only on the post-rate paypal-settlement.json")
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
		return errors.New("the '-i' or '--input' flag must be set")
	}
	if args.Currency == "" {
		return errors.New("the '-c' or '--currency' flag must be set")
	}

	payouts, err := ReadFiles(args.In)
	if err != nil {
		return err
	}

	rate, err := paypal.GetRate(ctx, args.Currency, args.Rate)
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
