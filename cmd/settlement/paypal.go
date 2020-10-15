package settlement

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/settlement"
	"github.com/brave-intl/bat-go/settlement/paypal"
	"github.com/brave-intl/bat-go/utils/closers"
	"github.com/gocarina/gocsv"
	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	// add complete and transform subcommand
	PaypalSettlementCmd.AddCommand(CompletePaypalSettlementCmd)
	PaypalSettlementCmd.AddCommand(TransformPaypalSettlementCmd)
	PaypalSettlementCmd.AddCommand(EmailPaypalSettlementCmd)

	// add this command as a settlement subcommand
	settlementCmd.AddCommand(PaypalSettlementCmd)

	// setup the flags

	// input (required by all)
	PaypalSettlementCmd.PersistentFlags().String("input", "",
		"the file or comma delimited list of files that should be utilized")
	cmd.Must(viper.BindPFlag("input", PaypalSettlementCmd.PersistentFlags().Lookup("input")))
	cmd.Must(viper.BindEnv("input", "INPUT"))
	cmd.Must(PaypalSettlementCmd.MarkPersistentFlagRequired("input"))

	// out (required by all with default)
	PaypalSettlementCmd.PersistentFlags().String("out", "./paypal-settlement",
		"the location of the file")
	cmd.Must(viper.BindPFlag("out", PaypalSettlementCmd.PersistentFlags().Lookup("out")))
	cmd.Must(viper.BindEnv("out", "OUT"))

	// currency (required by transform)
	TransformPaypalSettlementCmd.PersistentFlags().String("currency", "",
		"a currency must be set")
	cmd.Must(viper.BindPFlag("currency", TransformPaypalSettlementCmd.PersistentFlags().Lookup("currency")))
	cmd.Must(viper.BindEnv("currency", "CURRENCY"))
	cmd.Must(TransformPaypalSettlementCmd.MarkPersistentFlagRequired("currency"))

	// txnID (required by complete)
	CompletePaypalSettlementCmd.PersistentFlags().String("txn-id", "",
		"the completed mass pay transaction id")
	cmd.Must(viper.BindPFlag("txn-id", PaypalSettlementCmd.PersistentFlags().Lookup("txn-id")))
	cmd.Must(viper.BindEnv("txn-id", "TXN_ID"))
	cmd.Must(CompletePaypalSettlementCmd.MarkPersistentFlagRequired("txn-id"))

	// rate
	TransformPaypalSettlementCmd.PersistentFlags().Float64("rate", 0,
		"the rate to compute the currency conversion")
	cmd.Must(viper.BindPFlag("rate", TransformPaypalSettlementCmd.PersistentFlags().Lookup("rate")))
	cmd.Must(viper.BindEnv("rate", "RATE"))
}

// PaypalEmailTemplate performs template replacement of date fields in emails
func PaypalEmailTemplate(inPath string, outPath string) (err error) {
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
	// PaypalSettlementCmd is the paypal command
	PaypalSettlementCmd = &cobra.Command{
		Use:   "paypal",
		Short: "provides paypal settlement",
	}

	// EmailPaypalSettlementCmd provides population of a templated email
	EmailPaypalSettlementCmd = &cobra.Command{
		Use:   "email",
		Short: "provides population of a templated email",
		Run:   cmd.Perform("email", EmailPaypalSettlement),
	}

	// CompletePaypalSettlementCmd provides completion of paypal settlement
	CompletePaypalSettlementCmd = &cobra.Command{
		Use:   "complete",
		Short: "provides completion of paypal settlement",
		Run:   cmd.Perform("complete", CompletePaypalSettlement),
	}

	// TransformPaypalSettlementCmd provides transform of paypal settlement for mass pay
	TransformPaypalSettlementCmd = &cobra.Command{
		Use:   "transform",
		Short: "provides transform of paypal settlement for mass pay",
		Run:   cmd.Perform("transform", TransformPaypalSettlement),
	}
)

// EmailPaypalSettlement create the email to send to the
func EmailPaypalSettlement(cmd *cobra.Command, args []string) error {
	return PaypalEmailTemplate(viper.GetString("input"), viper.GetString("out"))
}

// TransformPaypalSettlement transforms a paypal settlement
func TransformPaypalSettlement(cmd *cobra.Command, args []string) error {
	payouts, err := settlement.ReadFiles(strings.Split(viper.GetString("input"), ","))
	if err != nil {
		return err
	}

	return PaypalTransformForMassPay(
		cmd.Context(),
		payouts,
		viper.GetString("currency"),
		decimal.NewFromFloat(viper.GetFloat64("rate")),
		viper.GetString("out"),
	)
}

// CompletePaypalSettlement added complete paypal settlement
func CompletePaypalSettlement(cmd *cobra.Command, args []string) error {
	input := viper.GetString("input")
	out := viper.GetString("out")
	txnID := viper.GetString("txn-id")
	return PaypalCompleteSettlement(
		input,
		out,
		txnID,
	)
}

// PaypalCompleteSettlement marks the settlement file as complete
func PaypalCompleteSettlement(inPath string, outPath string, txnID string) error {
	fmt.Println("RUNNING: complete")
	if outPath == "./paypal-settlement" {
		// use a file with extension if none is passed
		outPath = "./paypal-settlement-complete.json"
	}
	payouts, err := settlement.ReadFiles(strings.Split(inPath, ","))
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
	err = PaypalWriteTransactions(outPath, payouts)
	if err != nil {
		return err
	}
	return nil
}

// PaypalTransformArgs are the args required for the transform command
type PaypalTransformArgs struct {
	In       string
	Currency string
	Auth     string
	Rate     decimal.Decimal
	Out      string
}

// PaypalWriteTransactions writes settlement transactions to a json file
func PaypalWriteTransactions(outPath string, metadata *[]settlement.Transaction) error {
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(outPath, data, 0600)
}

// PaypalWriteMassPayCSV writes a csv for using with Paypal web mass payments
func PaypalWriteMassPayCSV(ctx context.Context, outPath string, metadata *[]paypal.Metadata) error {
	rows := []*paypal.MassPayRow{}
	total := decimal.NewFromFloat(0)
	logger := zerolog.Ctx(ctx)
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
	logger.UpdateContext(func(c zerolog.Context) zerolog.Context {
		return c.Int("payouts", len(rows)).
			Str("total", total.String()).
			Str("currency", currency)
	})

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

// PaypalTransformForMassPay starts the process to transform a settlement into a mass pay csv
func PaypalTransformForMassPay(ctx context.Context, payouts *[]settlement.Transaction, currency string, rate decimal.Decimal, out string) error {
	rate, err := paypal.GetRate(ctx, currency, rate)
	if err != nil {
		return err
	}

	txs, err := paypal.CalculateTransactionAmounts(currency, rate, payouts)
	if err != nil {
		return err
	}

	err = PaypalWriteTransactions(out+".json", txs)
	if err != nil {
		return err
	}

	metadata, err := paypal.MergeAndTransformPayouts(txs)
	if err != nil {
		return err
	}

	err = PaypalWriteMassPayCSV(ctx, out+".csv", metadata)
	if err != nil {
		return err
	}
	return nil
}
