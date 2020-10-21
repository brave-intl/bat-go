package wallets

import (
	"context"
	"crypto"
	"encoding/hex"
	"errors"
	"os"

	"github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/brave-intl/bat-go/utils/prompt"
	"github.com/brave-intl/bat-go/utils/vaultsigner"
	"github.com/brave-intl/bat-go/utils/wallet"
	"github.com/brave-intl/bat-go/utils/wallet/provider/uphold"
	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/crypto/ed25519"
)

var (
	// TransferFundsCmd transfer funds command
	TransferFundsCmd = &cobra.Command{
		Use:   "transfer-funds",
		Short: "transfers funds from one wallet to another",
		Run:   cmd.Perform("transfer funds", RunTransferFunds),
	}
)

func init() {
	WalletsCmd.AddCommand(
		TransferFundsCmd,
	)

	TransferFundsCmd.Flags().String("currency", "BAT",
		"currency for transfer")
	cmd.Must(viper.BindPFlag("currency", TransferFundsCmd.Flags().Lookup("currency")))

	TransferFundsCmd.Flags().String("from", "",
		"vault name for the source wallet")
	cmd.Must(viper.BindPFlag("from", TransferFundsCmd.Flags().Lookup("from")))
	cmd.Must(TransferFundsCmd.MarkFlagRequired("from"))

	TransferFundsCmd.Flags().String("note", "",
		"optional note for the transfer")
	cmd.Must(viper.BindPFlag("note", TransferFundsCmd.Flags().Lookup("note")))

	TransferFundsCmd.Flags().Bool("oneshot", false,
		"submit and commit without confirming")
	cmd.Must(viper.BindPFlag("oneshot", TransferFundsCmd.Flags().Lookup("oneshot")))

	TransferFundsCmd.Flags().String("to", "",
		"destination wallet address")
	cmd.Must(viper.BindPFlag("to", TransferFundsCmd.Flags().Lookup("to")))
	cmd.Must(TransferFundsCmd.MarkFlagRequired("to"))

	TransferFundsCmd.Flags().String("value", "",
		"amount to transfer [float or all]")
	cmd.Must(viper.BindPFlag("value", TransferFundsCmd.Flags().Lookup("value")))
	cmd.Must(TransferFundsCmd.MarkFlagRequired("value"))

	TransferFundsCmd.Flags().String("provider", "uphold",
		"provider for the source wallet")
	cmd.Must(viper.BindPFlag("provider", TransferFundsCmd.Flags().Lookup("provider")))

	TransferFundsCmd.Flags().Bool("usevault", false,
		"should signer should pull from vault")
	cmd.Must(viper.BindPFlag("usevault", TransferFundsCmd.Flags().Lookup("usevault")))
}

// RunTransferFunds moves funds from one wallet to another
func RunTransferFunds(command *cobra.Command, args []string) error {
	value, err := command.Flags().GetString("value")
	if err != nil {
		return err
	}
	from, err := command.Flags().GetString("from")
	if err != nil {
		return err
	}
	to, err := command.Flags().GetString("to")
	if err != nil {
		return err
	}
	currency, err := command.Flags().GetString("currency")
	if err != nil {
		return err
	}
	note, err := command.Flags().GetString("note")
	if err != nil {
		return err
	}
	oneshot, err := command.Flags().GetBool("oneshot")
	if err != nil {
		return err
	}
	usevault, err := command.Flags().GetBool("usevault")
	if err != nil {
		return err
	}

	ctx := command.Context()
	return TransferFunds(
		ctx,
		from,
		to,
		value,
		currency,
		note,
		oneshot,
		usevault,
	)
}

func pullRequisiteSecrets(from string, usevault bool) (string, crypto.Signer, error) {
	if usevault {
		return pullRequisiteSecretsFromVault(from)
	}
	return pullRequisiteSecretsFromEnv(from)
}

func pullRequisiteSecretsFromEnv(from string) (string, crypto.Signer, error) {
	providerID := os.Getenv("UPHOLD_PROVIDER_ID")
	privateKey := os.Getenv("ED25519_PRIVATE_KEY")
	return providerID, ed25519.NewKeyFromSeed([]byte(privateKey)), nil
}

func pullRequisiteSecretsFromVault(from string) (string, *vaultsigner.Ed25519Signer, error) {
	wrappedClient, err := vaultsigner.Connect()
	if err != nil {
		return "", nil, err
	}

	response, err := wrappedClient.Client.Logical().Read("wallets/" + from)
	if err != nil {
		return "", nil, err
	}

	providerID, ok := response.Data["providerId"]
	if !ok {
		return "", nil, errors.New("invalid wallet name")
	}

	signer, err := wrappedClient.GenerateEd25519Signer(from)
	if err != nil {
		return "", signer, err
	}

	providerIDString := providerID.(string)
	return providerIDString, signer, nil
}

// TransferFunds transfers funds to a wallet
func TransferFunds(
	ctx context.Context,
	from string,
	to string,
	value string,
	currency string,
	note string,
	oneshot bool,
	usevault bool,
) error {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		_, logger = logging.SetupLogger(ctx)
	}
	valueDec, err := decimal.NewFromString(value)
	if value != "all" && (err != nil || valueDec.LessThanOrEqual(decimal.Zero)) {
		return errors.New("must pass --value greater than 0 or --value=all")
	}

	providerID, signer, err := pullRequisiteSecrets(from, usevault)
	if err != nil {
		return err
	}
	walletc := altcurrency.BAT

	var info wallet.Info
	info.PublicKey = hex.EncodeToString(signer.Public().(ed25519.PublicKey))
	info.Provider = "uphold"
	info.ProviderID = providerID
	{
		tmp := walletc
		info.AltCurrency = &tmp
	}

	var pubKey httpsignature.Ed25519PubKey
	pubKey, err = hex.DecodeString(info.PublicKey)
	if err != nil {
		return err
	}

	w := &uphold.Wallet{Info: info, PrivKey: signer, PubKey: pubKey}

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

		logger.Info().
			Str("id", submitInfo.ID).
			Str("from", from).
			Str("to", to).
			Str("currency", currency).
			Str("amount", altc.FromProbi(valueProbi).String()).
			Msg("will transfer")

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
			logger.Error().Err(err).Msg("error confirming")
			return err
		}

		upholdInfo, err := w.GetTransaction(submitInfo.ID)
		if err != nil {
			return err
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
