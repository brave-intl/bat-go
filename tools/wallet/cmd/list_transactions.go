package cmd

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	cmdutils "github.com/brave-intl/bat-go/cmd"
	rootcmd "github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/libs/altcurrency"
	"github.com/brave-intl/bat-go/libs/wallet"
	"github.com/brave-intl/bat-go/libs/wallet/provider"
	"github.com/spf13/cobra"
)

const (
	dateFormat = "2006-01-02T15:04:05-0700"
)

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
		Run:   rootcmd.Perform("list transactions", RunListTransactions),
	}
)

func init() {
	WalletsCmd.AddCommand(ListTransactionsCmd)

	listTransactionsBuilder := cmdutils.NewFlagBuilder(ListTransactionsCmd)

	listTransactionsBuilder.Flag().Bool("csv", false,
		"the output file should be csv").
		Bind("csv").
		Require()

	listTransactionsBuilder.Flag().Bool("signed", false,
		"signed value depending on transaction direction").
		Bind("signed").
		Require()

	listTransactionsBuilder.Flag().Int("limit", 50,
		"limit number of transactions returned").
		Bind("limit").
		Require()

	listTransactionsBuilder.Flag().String("start-date", "none",
		"only include transactions after this datetime [ISO 8601]").
		Bind("start-date").
		Require()

	listTransactionsBuilder.Flag().String("provider", "uphold",
		"provider for the source wallet").
		Bind("provider").
		Require()
}

// RunListTransactions runs the list transactions command
func RunListTransactions(cmd *cobra.Command, args []string) error {
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

	txns, err := w.ListTransactions(ctx, limit, startDate)
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
