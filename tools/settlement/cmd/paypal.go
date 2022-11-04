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

	cmdutils "github.com/brave-intl/bat-go/cmd"
	rootcmd "github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/libs/custodian"

	"github.com/brave-intl/bat-go/libs/closers"
	"github.com/brave-intl/bat-go/tools/settlement"
	"github.com/brave-intl/bat-go/tools/settlement/paypal"
	"github.com/gocarina/gocsv"
	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"
	"github.com/spf13/cobra"
)

func init() {
	// add complete and transform subcommand
	PaypalSettlementCmd.AddCommand(CompletePaypalSettlementCmd)
	PaypalSettlementCmd.AddCommand(TransformPaypalSettlementCmd)
	PaypalSettlementCmd.AddCommand(EmailPaypalSettlementCmd)

	// add this command as a settlement subcommand
	SettlementCmd.AddCommand(PaypalSettlementCmd)

	// setup the flags
	completeBuilder := cmdutils.NewFlagBuilder(CompletePaypalSettlementCmd)
	transformBuilder := cmdutils.NewFlagBuilder(TransformPaypalSettlementCmd)
	emailBuilder := cmdutils.NewFlagBuilder(EmailPaypalSettlementCmd)
	transformEmailCompleteBuilder := completeBuilder.Concat(transformBuilder, emailBuilder)

	transformEmailCompleteBuilder.Flag().String("input", "",
		"the file or comma delimited list of files that should be utilized").
		Env("INPUT").
		Bind("input").
		Require()

	transformEmailCompleteBuilder.Flag().String("out", "./paypal-settlement",
		"the location of the file to write out").
		Env("OUT").
		Bind("out").
		Require()

	transformBuilder.Flag().String("currency", "",
		"a currency must be set (usually JPY)").
		Env("CURRENCY").
		Bind("currency").
		Require()

	completeBuilder.Flag().String("txn-id", "",
		"the completed mass pay transaction id").
		Env("TXN_ID").
		Bind("txn-id").
		Require()

	transformBuilder.Flag().Float64("rate", 0,
		"a currency must be set (usually JPY)").
		Bind("rate").
		Env("RATE")
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
		Run:   rootcmd.Perform("email", EmailPaypalSettlement),
	}

	// CompletePaypalSettlementCmd provides completion of paypal settlement
	CompletePaypalSettlementCmd = &cobra.Command{
		Use:   "complete",
		Short: "provides completion of paypal settlement",
		Run:   rootcmd.Perform("complete", CompletePaypalSettlement),
	}

	// TransformPaypalSettlementCmd provides transform of paypal settlement for mass pay
	TransformPaypalSettlementCmd = &cobra.Command{
		Use:   "transform",
		Short: "provides transform of paypal settlement for mass pay",
		Run:   rootcmd.Perform("transform", RunTransformPaypalSettlement),
	}
)

// EmailPaypalSettlement create the email to send to the
func EmailPaypalSettlement(cmd *cobra.Command, args []string) error {
	input, err := cmd.Flags().GetString("input")
	if err != nil {
		return err
	}
	out, err := cmd.Flags().GetString("out")
	if err != nil {
		return err
	}
	return PaypalEmailTemplate(input, out)
}

// RunTransformPaypalSettlement transforms a paypal settlement
func RunTransformPaypalSettlement(cmd *cobra.Command, args []string) error {
	input, err := cmd.Flags().GetString("input")
	if err != nil {
		return err
	}
	payouts, err := settlement.ReadFiles(strings.Split(input, ","))
	if err != nil {
		return err
	}
	currency, err := cmd.Flags().GetString("currency")
	if err != nil {
		return err
	}
	rate, err := cmd.Flags().GetFloat64("rate")
	if err != nil {
		return err
	}
	out, err := cmd.Flags().GetString("out")
	if err != nil {
		return err
	}

	return PaypalTransformForMassPay(
		cmd.Context(),
		payouts,
		currency,
		decimal.NewFromFloat(rate),
		out,
	)
}

// CompletePaypalSettlement added complete paypal settlement
func CompletePaypalSettlement(cmd *cobra.Command, args []string) error {
	input, err := cmd.Flags().GetString("input")
	if err != nil {
		return err
	}
	out, err := cmd.Flags().GetString("out")
	if err != nil {
		return err
	}
	txnID, err := cmd.Flags().GetString("txn-id")
	if err != nil {
		return err
	}

	if out == "./paypal-settlement" {
		// use a file with extension if none is passed
		out = "./paypal-settlement-complete.json"
	}
	payouts, err := settlement.ReadFiles(strings.Split(input, ","))
	if err != nil {
		return err
	}
	payouts, err = PaypalCompleteSettlement(
		payouts,
		txnID,
	)
	if err != nil {
		return err
	}
	err = PaypalWriteTransactions(out, payouts)
	if err != nil {
		return err
	}
	return nil
}

// PaypalCompleteSettlement marks the settlement file as complete
func PaypalCompleteSettlement(payouts *[]custodian.Transaction, txnID string) (*[]custodian.Transaction, error) {
	for i, payout := range *payouts {
		if payout.WalletProvider != "paypal" {
			return nil, errors.New("error, non-paypal payment included.\nThis command should be called only on the filtered paypal-settlement.json")
		}
		if !payout.Amount.GreaterThan(decimal.Zero) {
			return nil, errors.New("error, non-zero payment included.\nThis command should be called only on the post-rate paypal-settlement.json")
		}
		payout.Status = "complete"
		payout.ProviderID = txnID
		(*payouts)[i] = payout
	}
	return payouts, nil
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
func PaypalWriteTransactions(outPath string, metadata *[]custodian.Transaction) error {
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
	defer closers.Panic(ctx, f)
	_, err = f.WriteString(data)
	if err != nil {
		return err
	}
	return nil
}

// PaypalTransformForMassPay starts the process to transform a settlement into a mass pay csv
func PaypalTransformForMassPay(ctx context.Context, payouts *[]custodian.Transaction, currency string, rate decimal.Decimal, out string) error {
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
