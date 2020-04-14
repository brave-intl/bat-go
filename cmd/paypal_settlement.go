package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	input    string
	currency string
	txnID    string
	auth     string
	rate     float64
	out      string
	template string
)

func init() {
	settlementCmd.AddCommand(paypalSettlementCmd)

	// setup the flags

	// input
	paypalSettlementCmd.PersistentFlags().StringVarP(&input, "input", "i", "",
		"the file or comma delimited list of files that should be utilized")
	viper.BindPFlag("input", paypalSettlementCmd.PersistentFlags().Lookup("input"))
	viper.BindEnv("input", "INPUT")
	// currency
	paypalSettlementCmd.PersistentFlags().StringVarP(&currency, "currency", "c", "",
		"a currency must be set")
	viper.BindPFlag("currency", paypalSettlementCmd.PersistentFlags().Lookup("currency"))
	viper.BindEnv("currency", "CURRENCY")
	// txnID
	paypalSettlementCmd.PersistentFlags().StringVarP(&txnID, "txn-id", "t", "",
		"the completed mass pay transaction id")
	viper.BindPFlag("txn-id", paypalSettlementCmd.PersistentFlags().Lookup("txn-id"))
	viper.BindEnv("txn-id", "TXN_ID")
	// rate-auth
	paypalSettlementCmd.PersistentFlags().StringVarP(&auth, "rate-auth", "a", "",
		"the rate auth service")
	viper.BindPFlag("rate-auth", paypalSettlementCmd.PersistentFlags().Lookup("rate"))
	viper.BindEnv("rate-auth", "RATE_AUTH")
	// rate
	paypalSettlementCmd.PersistentFlags().Float64VarP(&rate, "rate", "r", 0,
		"the rate to compute the currency conversion")
	viper.BindPFlag("rate", paypalSettlementCmd.PersistentFlags().Lookup("rate"))
	viper.BindEnv("rate", "RATE")
	// out
	paypalSettlementCmd.PersistentFlags().StringVarP(&out, "out", "o", "./paypal-settlement",
		"the location of the file")
	viper.BindPFlag("out", paypalSettlementCmd.PersistentFlags().Lookup("out"))
	viper.BindEnv("out", "OUT")
	// template
	paypalSettlementCmd.PersistentFlags().StringVarP(&template, "template", "T", "",
		"the location of the formatting template file")
	viper.BindPFlag("template", paypalSettlementCmd.PersistentFlags().Lookup("template"))
	viper.BindEnv("template", "TEMPLATE")
}

var paypalSettlementCmd = &cobra.Command{
	Use:   "paypal",
	Short: "provides paypal settlement",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("paypal settlement command")
	},
}
