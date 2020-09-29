package vault

import (
	"errors"

	"github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/prompt"
	"github.com/brave-intl/bat-go/utils/vaultsigner"
	"github.com/brave-intl/bat-go/utils/wallet"
	"github.com/brave-intl/bat-go/utils/wallet/provider/uphold"
	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// TransferFundsCmd transfer funds command
	TransferFundsCmd = &cobra.Command{
		Use:   "transfer-funds",
		Short: "transfers funds from one wallet to another",
		Run:   cmd.Perform("transfer funds", TransferFunds),
	}
)

func init() {
	VaultCmd.AddCommand(
		TransferFundsCmd,
	)

	TransferFundsCmd.PersistentFlags().String("currency", "BAT",
		"currency for transfer")
	cmd.Must(viper.BindPFlag("currency", TransferFundsCmd.PersistentFlags().Lookup("currency")))

	TransferFundsCmd.PersistentFlags().String("from", "",
		"vault name for the source wallet")
	cmd.Must(viper.BindPFlag("from", TransferFundsCmd.PersistentFlags().Lookup("from")))
	cmd.Must(TransferFundsCmd.MarkPersistentFlagRequired("from"))

	TransferFundsCmd.PersistentFlags().String("note", "",
		"optional note for the transfer")
	cmd.Must(viper.BindPFlag("note", TransferFundsCmd.PersistentFlags().Lookup("note")))

	TransferFundsCmd.PersistentFlags().Bool("oneshot", false,
		"submit and commit without confirming")
	cmd.Must(viper.BindPFlag("oneshot", TransferFundsCmd.PersistentFlags().Lookup("oneshot")))

	TransferFundsCmd.PersistentFlags().String("to", "",
		"destination wallet address")
	cmd.Must(viper.BindPFlag("to", TransferFundsCmd.PersistentFlags().Lookup("to")))
	cmd.Must(TransferFundsCmd.MarkPersistentFlagRequired("to"))

	TransferFundsCmd.PersistentFlags().String("value", "",
		"amount to transfer [float or all]")
	cmd.Must(viper.BindPFlag("value", TransferFundsCmd.PersistentFlags().Lookup("value")))
}

// TransferFunds moves funds from one wallet to another
func TransferFunds(command *cobra.Command, args []string) error {
	value := viper.GetString("value")
	from := viper.GetString("from")
	to := viper.GetString("to")
	currency := viper.GetString("currency")
	note := viper.GetString("note")
	oneshot := viper.GetBool("oneshot")
	logger, err := appctx.GetLogger(command.Context())
	cmd.Must(err)

	valueDec, err := decimal.NewFromString(value)
	if value != "all" && (err != nil || valueDec.LessThan(decimal.Zero)) {
		return errors.New("must pass -value greater than 0 or -value all")
	}

	if len(from) == 0 || len(to) == 0 {
		return errors.New("must pass non-empty -from and -to")
	}

	wrappedClient, err := vaultsigner.Connect()
	if err != nil {
		return err
	}

	response, err := wrappedClient.Client.Logical().Read("wallets/" + from)
	if err != nil {
		return err
	}
	logger.Info().Msgf("%#v\n", response)

	providerID, ok := response.Data["providerId"]
	if !ok {
		return errors.New("invalid wallet name")
	}

	signer, err := wrappedClient.GenerateEd25519Signer(from)
	if err != nil {
		return err
	}

	walletc := altcurrency.BAT

	var info wallet.Info
	info.PublicKey = signer.String()
	info.Provider = "uphold"
	info.ProviderID = providerID.(string)
	{
		tmp := walletc
		info.AltCurrency = &tmp
	}

	w := &uphold.Wallet{Info: info, PrivKey: signer, PubKey: signer}

	altc, err := altcurrency.FromString(currency)
	if err != nil {
		return err
	}

	var valueProbi decimal.Decimal
	var balance *wallet.Balance

	if walletc == altc {
		balance, err = w.GetBalance(true)
		if err != nil {
			return err
		}
	}

	if value == "all" {
		if walletc == altc {
			valueProbi = balance.SpendableProbi
		} else {
			return errors.New("Sending all funds not available for currencies other than the wallet currency")
		}
	} else {
		valueProbi = altc.ToProbi(valueDec)
		if walletc == altc && balance.SpendableProbi.LessThan(valueProbi) {
			return errors.New("Insufficient funds in wallet")
		}
	}

	signedTx, err := w.PrepareTransaction(altc, valueProbi, to, note)
	if err != nil {
		return err
	}
	for {
		submitInfo, err := w.SubmitTransaction(signedTx, oneshot)
		if err != nil {
			return err
		}
		if oneshot {
			logger.Info().Msg("transfer complete")
			break
		}

		logger.Info().Msgf("Submitted quote for transfer, id: %s\n", submitInfo.ID)

		logger.Info().Msgf("Will transfer %s %s from %s to %s\n", altc.FromProbi(valueProbi).String(), currency, from, to)

		log.Printf("Continue? ")
		resp, err := prompt.Bool()
		if err != nil {
			return err
		}
		if !resp {
			return errors.New("exiting")
		}

		_, err = w.ConfirmTransaction(submitInfo.ID)
		if err != nil {
			logger.Info().Msgf("error confirming: %s\n", err)
		}

		upholdInfo, err := w.GetTransaction(submitInfo.ID)
		if err != nil {
			log.Fatalln(err)
		}
		if upholdInfo.Status == "completed" {
			logger.Info().Msg("transfer complete")
			break
		}

		log.Printf("Confirmation did not appear to go through, retry?")
		resp, err = prompt.Bool()
		if err != nil {
			return err
		}
		if !resp {
			return errors.New("exiting")
		}
	}
	return nil
}
