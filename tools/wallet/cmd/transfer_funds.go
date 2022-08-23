package cmd

import (
	"bufio"
	"context"
	"crypto"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"

	cmdutils "github.com/brave-intl/bat-go/cmd"
	rootcmd "github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/libs/altcurrency"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/passphrase"
	"github.com/brave-intl/bat-go/libs/prompt"
	"github.com/brave-intl/bat-go/libs/wallet"
	"github.com/brave-intl/bat-go/libs/wallet/provider/uphold"
	vaultsigner "github.com/brave-intl/bat-go/tools/vault/signer"
	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ed25519"
)

var (
	// TransferFundsCmd transfer funds command
	TransferFundsCmd = &cobra.Command{
		Use:   "transfer-funds",
		Short: "transfers funds from one wallet to another",
		Run:   rootcmd.Perform("transfer funds", RunTransferFunds),
	}
	// WalletsCmd root wallets command
	WalletsCmd = &cobra.Command{
		Use:   "wallet",
		Short: "provides wallets micro-service entrypoint",
	}
)

func init() {

	// add this command as a serve subcommand
	rootcmd.RootCmd.AddCommand(WalletsCmd)

	WalletsCmd.AddCommand(
		TransferFundsCmd,
	)

	transferFundsBuilder := cmdutils.NewFlagBuilder(TransferFundsCmd)
	transferFundsBuilder.Flag().String("currency", "BAT",
		"currency for transfer").
		Bind("currency")

	transferFundsBuilder.Flag().String("from", "",
		"vault name for the source wallet").
		Bind("from").
		Require()

	transferFundsBuilder.Flag().String("note", "",
		"optional note for the transfer").
		Bind("note")

	transferFundsBuilder.Flag().String("purpose", "",
		"purpose for the transfer, required for value > $3000").
		Bind("purpose")

	transferFundsBuilder.Flag().String("beneficiary", "",
		"JSON formatted beneficiary for the transfer, required for value > $3000").
		Bind("beneficiary")

	transferFundsBuilder.Flag().Bool("oneshot", false,
		"submit and commit without confirming").
		Bind("oneshot")

	transferFundsBuilder.Flag().String("to", "",
		"destination wallet address").
		Bind("to").
		Require()

	transferFundsBuilder.Flag().String("value", "",
		"amount to transfer [float or all]").
		Bind("value").
		Require()

	transferFundsBuilder.Flag().String("provider", "uphold",
		"provider for the source wallet").
		Bind("provider")

	transferFundsBuilder.Flag().Bool("usevault", false,
		"should signer should pull from vault").
		Bind("usevault")
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
	purpose, err := command.Flags().GetString("purpose")
	if err != nil {
		return err
	}
	beneficiaryJSON, err := command.Flags().GetString("beneficiary")
	if err != nil {
		return err
	}
	var beneficiary *uphold.Beneficiary
	if len(beneficiaryJSON) > 0 {
		beneficiary = &uphold.Beneficiary{}
		err := json.Unmarshal([]byte(beneficiaryJSON), beneficiary)
		if err != nil {
			return err
		}
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
		purpose,
		beneficiary,
		oneshot,
		usevault,
	)
}

func pullRequisiteSecrets(from string, usevault bool) (string, crypto.Signer, error) {
	if usevault {
		return pullRequisiteSecretsFromVault(from)
	}
	providerID, privateKey, err := pullRequisiteSecretsFromEnv(from)
	if privateKey == nil {
		// Fallback to prompting for a seed phrase
		return pullRequisiteSecretsFromPrompt(from)
	}
	return providerID, privateKey, err
}

func pullRequisiteSecretsFromPrompt(from string) (string, crypto.Signer, error) {
	log.Println("Enter your recovery phrase:")
	reader := bufio.NewReader(os.Stdin)
	recoveryPhrase, err := reader.ReadString('\n')
	if err != nil {
		return "", nil, err
	}

	seed, err := passphrase.ToBytes32(recoveryPhrase)
	if err != nil {
		return "", nil, err
	}

	key, err := passphrase.DeriveSigningKeysFromSeed(seed, passphrase.LedgerHKDFSalt)
	if err != nil {
		return "", nil, err
	}

	return from, key, nil
}

func pullRequisiteSecretsFromEnv(from string) (string, crypto.Signer, error) {
	privateKeyHex := os.Getenv("ED25519_PRIVATE_KEY")

	if len(privateKeyHex) == 0 {
		return "", nil, nil
	}

	var privKey ed25519.PrivateKey
	privKey, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return "", nil, errors.New("Key material must be passed as hex")
	}

	return from, privKey, nil
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
	purpose string,
	beneficiary *uphold.Beneficiary,
	oneshot bool,
	usevault bool,
) error {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		_, logger = logging.SetupLogger(ctx)
	}
	logger.Debug().Msg("debug enabled")
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

	w, err := uphold.New(ctx, info, signer, pubKey)
	if err != nil {
		return err
	}

	altc, err := altcurrency.FromString(currency)
	if err != nil {
		return err
	}

	var valueProbi decimal.Decimal
	var balance *wallet.Balance

	if walletc == altc {
		balance, err = w.GetBalance(ctx, true)
		if err != nil {
			return err
		}
	}

	if value == "all" {
		if walletc == altc {
			valueProbi = balance.SpendableProbi
		} else {
			return errors.New("sending all funds not available for currencies other than the wallet currency")
		}
	} else {
		valueProbi = altc.ToProbi(valueDec)
		if walletc == altc && balance.SpendableProbi.LessThan(valueProbi) {
			return errors.New("insufficient funds in wallet")
		}
	}

	signedTx, err := w.PrepareTransaction(altc, valueProbi, to, note, purpose, beneficiary)
	if err != nil {
		return err
	}
	for {
		submitInfo, err := w.SubmitTransaction(ctx, signedTx, oneshot)
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

		_, err = w.ConfirmTransaction(ctx, submitInfo.ID)
		if err != nil {
			logger.Error().Err(err).Msg("error confirming")
			return err
		}

		upholdInfo, err := w.GetTransaction(ctx, submitInfo.ID)
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
