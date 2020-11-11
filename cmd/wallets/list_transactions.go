package wallets

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/wallet"
	"github.com/brave-intl/bat-go/utils/wallet/provider"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	dateFormat = "2006-01-02T15:04:05-0700"
)

// var verbose = flag.Bool("v", false, "verbose output")
// var csvOut = flag.Bool("csv", false, "csv output")
// var limit = flag.Int("limit", 50, "limit number of transactions returned")
// var startDateStr = flag.String("start-date", "none", "only include transactions after this datetime  [ISO 8601]")
// var walletProvider = flag.String("provider", "uphold", "provider for the source wallet")
// var signed = flag.Bool("signed", false, "signed value depending on transaction direction")

var (
	// ListTransactionsCmd is a command to list transactions from a given wallet
	ListTransactionsCmd = &cobra.Command{
		Use:   "list-transactions",
		Short: "lists a transactions from a given wallet",
		Run:   cmd.Perform("list transactions", RunListTransactions),
	}
)

func init() {
	WalletsCmd.AddCommand(ListTransactionsCmd)

	ListTransactionsCmd.Flags().Bool("verbose", false,
		"how verbose logging should be")
	cmd.Must(viper.BindPFlag("verbose", ListTransactionsCmd.Flags().Lookup("verbose")))

	ListTransactionsCmd.Flags().Bool("csv", false,
		"the output file should be csv")
	cmd.Must(viper.BindPFlag("csv", ListTransactionsCmd.Flags().Lookup("csv")))
	cmd.Must(ListTransactionsCmd.MarkFlagRequired("csv"))

	ListTransactionsCmd.Flags().Bool("signed", false,
		"signed value depending on transaction direction")
	cmd.Must(viper.BindPFlag("signed", ListTransactionsCmd.Flags().Lookup("signed")))
	cmd.Must(ListTransactionsCmd.MarkFlagRequired("signed"))

	ListTransactionsCmd.Flags().Int("limit", 50,
		"limit number of transactions returned")
	cmd.Must(viper.BindPFlag("limit", ListTransactionsCmd.Flags().Lookup("limit")))
	cmd.Must(ListTransactionsCmd.MarkFlagRequired("limit"))

	ListTransactionsCmd.Flags().String("start-date", "none",
		"only include transactions after this datetime [ISO 8601]")
	cmd.Must(viper.BindPFlag("start-date", ListTransactionsCmd.Flags().Lookup("start-date")))
	cmd.Must(ListTransactionsCmd.MarkFlagRequired("start-date"))

	ListTransactionsCmd.Flags().String("provider", "uphold",
		"provider for the source wallet")
	cmd.Must(viper.BindPFlag("provider", ListTransactionsCmd.Flags().Lookup("provider")))
	cmd.Must(ListTransactionsCmd.MarkFlagRequired("provider"))
}

// RunListTransactions runs the list transactions command
func RunListTransactions(cmd *cobra.Command, args []string) error {
	verbose, err := cmd.Flags().GetBool("verbose")
	if err != nil {
		return err
	}
	csvOut, err := cmd.Flags().GetBool("csv")
	if err != nil {
		return err
	}
	signed, err := cmd.Flags().GetBool("signed")
	if err != nil {
		return err
	}
	limit, err := cmd.Flags().GetInt("limit")
	if err != nil {
		return err
	}
	startDateStr, err := cmd.Flags().GetString("start-date")
	if err != nil {
		return err
	}
	provider, err := cmd.Flags().GetString("provider")
	if err != nil {
		return err
	}
	return ListTransactions(
		cmd.Context(),
		args,
		verbose,
		csvOut,
		signed,
		limit,
		startDateStr,
		provider,
	)
}

// ListTransactions lists transactions
func ListTransactions(
	ctx context.Context,
	args []string,
	verbose bool,
	csvOut bool,
	signed bool,
	limit int,
	startDateStr string,
	walletProvider string,
) error {
	var err error
	startDate := time.Unix(0, 0)
	if startDateStr != "none" {
		startDate, err = time.Parse(dateFormat, startDateStr)
		if err != nil {
			return fmt.Errorf("%s is not a valid ISO 8601 datetime", startDateStr)
		}
	}

	walletc := altcurrency.BAT
	info := wallet.Info{
		Provider:    walletProvider,
		ProviderID:  flag.Args()[0],
		AltCurrency: &walletc,
	}
	w, err := provider.GetWallet(ctx, info)
	if err != nil {
		return err
	}

	txns, err := w.ListTransactions(limit, startDate)
	if err != nil {
		return err
	}

	sort.Sort(wallet.ByTime(txns))

	if csvOut {
		w := csv.NewWriter(os.Stdout)
		err = w.Write([]string{"id", "date", "description", "probi", "altcurrency", "source", "destination", "transferFee", "exchangeFee", "destAmount", "destCurrency"})
		if err != nil {
			return err
		}

		for i := 0; i < len(txns); i++ {
			t := txns[i]

			value := t.AltCurrency.FromProbi(t.Probi).String()
			if signed {
				if t.Source == info.ProviderID {
					value = "-" + value
				} else if t.Destination == info.ProviderID {
					value = "+" + value
				} else {
					panic("Could not determine direction of transaction")
				}
			}

			record := []string{
				t.ID,
				t.Time.String(),
				t.Note,
				value,
				t.AltCurrency.String(),
				t.Source,
				t.Destination,
				t.TransferFee.String(),
				t.ExchangeFee.String(),
				t.DestAmount.String(),
				t.DestCurrency,
			}
			if err := w.Write(record); err != nil {
				return fmt.Errorf("error writing record to csv: %s", err)
			}
		}

		w.Flush()

		if err := w.Error(); err != nil {
			return err
		}
	} else {
		for i := 0; i < len(txns); i++ {
			fmt.Printf("%s\n", txns[i])
		}
	}
	return nil
}
