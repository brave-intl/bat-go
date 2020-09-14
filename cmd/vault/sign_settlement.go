package vault

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/ioutil"
	"path/filepath"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/cmd"
	settlementcmd "github.com/brave-intl/bat-go/cmd/settlement"
	"github.com/brave-intl/bat-go/settlement"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/clients/gemini"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/vaultsigner"
	"github.com/brave-intl/bat-go/utils/wallet"
	"github.com/brave-intl/bat-go/utils/wallet/provider/uphold"
	"github.com/shopspring/decimal"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// SignSettlementCmd signs a settlement file's transactions
	SignSettlementCmd = &cobra.Command{
		Use:   "sign-settlement",
		Short: "sign settlement files using vault inputs",
		Run:   cmd.Perform("sign settlement", SignSettlement),
	}
	// the combination of provider + transaction type gives you the key
	// under which the vault secrets are located by default
	providerTransactionTypes = map[string][]string{
		"uphold": {"contribution", "referral"},
		"paypal": {"default"},
		"gemini": {"contribution", "referral"},
	}
	artifactGenerators = map[string]func(
		context.Context,
		string,
		*vaultsigner.WrappedClient,
		string,
		[]settlement.Transaction,
	) error{
		"uphold": createUpholdArtifact,
		"gemini": createGeminiArtifact,
		"paypal": createPaypalArtifact,
	}
)

func init() {
	VaultCmd.AddCommand(
		SignSettlementCmd,
	)

	// in -> the file to parse and sign according to each provider's setup. default: contributions.json
	SignSettlementCmd.PersistentFlags().String("in", "contributions.json",
		"input file path")
	cmd.Must(viper.BindPFlag("in", SignSettlementCmd.PersistentFlags().Lookup("in")))

	providers := []string{}
	for k := range providerTransactionTypes {
		providers = append(providers, k)
	}
	// providers -> the providers to parse out of the file and parse. default: uphold paypal gemini
	SignSettlementCmd.PersistentFlags().StringSlice("providers", providers,
		"providers to parse out of the given input files")
	cmd.Must(viper.BindPFlag("providers", SignSettlementCmd.PersistentFlags().Lookup("providers")))

	// jpyRate -> the providers to parse out of the file and parse. default: uphold paypal gemini
	SignSettlementCmd.PersistentFlags().Float64("jpyrate", 0.0,
		"jpyrate to use for paypal payouts")
	cmd.Must(viper.BindPFlag("jpyrate", SignSettlementCmd.PersistentFlags().Lookup("jpyrate")))
}

// SignSettlement runs the signing of a settlement
func SignSettlement(command *cobra.Command, args []string) error {
	providers := viper.GetStringSlice("providers")
	inputFile := viper.GetString("in")
	// append -signed to the filename
	outputFile := strings.TrimSuffix(inputFile, filepath.Ext(inputFile)) + "-signed.json"
	logger, err := appctx.GetLogger(command.Context())
	cmd.Must(err)
	// all settlements file
	settlementJSON, err := ioutil.ReadFile(inputFile)
	cmd.Must(err)

	var antifraudSettlements []settlement.AntifraudTransaction
	err = json.Unmarshal(settlementJSON, &antifraudSettlements)
	cmd.Must(err)

	settlementsByProviderAndWalletKey := divideSettlementsByWallet(antifraudSettlements)

	wrappedClient, err := vaultsigner.Connect()
	if err != nil {
		return err
	}

	for _, provider := range providers {
		for _, txType := range providerTransactionTypes[provider] {
			walletKey := provider + "-" + txType
			settlements := settlementsByProviderAndWalletKey[walletKey]
			if len(settlements) == 0 {
				continue
			}
			output := walletKey + "-" + outputFile
			secretKey := Config.GetWalletKey(walletKey)
			ctx := command.Context()
			log := logger.Info().
				Str("output", output).
				Str("provider", provider).
				Str("secretkey", secretKey).
				Int("settlements", len(settlements))
			err := artifactGenerators[provider](
				context.WithValue(ctx, appctx.LogEvent, log),
				output,
				wrappedClient,
				secretKey,
				settlements,
			)
			cmd.Must(err)
			log.Msg("created artifact")
		}
	}
	return nil
}

func divideSettlementsByWallet(antifraudTxs []settlement.AntifraudTransaction) map[string][]settlement.Transaction {
	settlementTransactionsByWallet := make(map[string][]settlement.Transaction)

	for _, antifraudTx := range antifraudTxs {
		tx := antifraudTx.ToTransaction()

		provider := tx.WalletProvider
		wallet := tx.Type
		if provider == "paypal" {
			// might as well go into one (default)
			wallet = providerTransactionTypes[provider][0]
		}
		// which secret values to use to sign (paypal-default, uphold-referral, gemini-contribution)
		walletKey := provider + "-" + wallet
		// append to the nested structure
		settlementTransactionsByWallet[walletKey] = append(
			settlementTransactionsByWallet[walletKey],
			tx,
		)
	}
	return settlementTransactionsByWallet
}

func createUpholdArtifact(
	ctx context.Context,
	outputFile string,
	wrappedClient *vaultsigner.WrappedClient,
	walletKey string,
	upholdOnlySettlements []settlement.Transaction,
) error {
	response, err := wrappedClient.Client.Logical().Read("wallets/" + walletKey)
	if err != nil {
		return err
	}

	providerID, ok := response.Data["providerId"]
	if !ok {
		return errors.New("invalid wallet name")
	}

	signer, err := wrappedClient.GenerateEd25519Signer(walletKey)
	if err != nil {
		return err
	}

	var info wallet.Info
	info.PublicKey = signer.String()
	info.Provider = "uphold"
	info.ProviderID = providerID.(string)
	{
		tmp := altcurrency.BAT
		info.AltCurrency = &tmp
	}
	settlementWallet := &uphold.Wallet{Info: info, PrivKey: signer, PubKey: signer}

	err = settlement.PrepareTransactions(settlementWallet, upholdOnlySettlements)
	if err != nil {
		return err
	}

	state := settlement.State{WalletInfo: settlementWallet.Info, Transactions: upholdOnlySettlements}

	out, err := json.MarshalIndent(state, "", "    ")
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(outputFile, out, 0400)
	if err != nil {
		return err
	}
	return nil
}

func createGeminiArtifact(
	ctx context.Context,
	outputFile string,
	wrappedClient *vaultsigner.WrappedClient,
	walletKey string,
	geminiOnlySettlements []settlement.Transaction,
) error {
	response, err := wrappedClient.Client.Logical().Read("wallets/" + walletKey)
	if err != nil {
		return err
	}
	oauthClientID := response.Data["clientid"].(string)
	// group transactions (500 at a time)
	privatePayloads, err := settlementcmd.GeminiTransformTransactions(ctx, oauthClientID, geminiOnlySettlements)
	if err != nil {
		return err
	}
	// leave enough space to increment nonce
	<-time.After(time.Microsecond)
	privateRequests, err := signGeminiRequests(
		wrappedClient,
		walletKey,
		privatePayloads,
	)
	if err != nil {
		return err
	}
	// serialize requests to be sent in next step
	out, err := json.Marshal(privateRequests)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(outputFile, out, 0400)
	if err != nil {
		return err
	}
	return nil
}

func signGeminiRequests(
	wrappedClient *vaultsigner.WrappedClient,
	walletKey string,
	privateRequests *[][]gemini.PayoutPayload,
) (*[]gemini.PrivateRequestSequence, error) {
	response, err := wrappedClient.Client.Logical().Read("wallets/" + walletKey)
	if err != nil {
		return nil, err
	}
	clientID := response.Data["clientid"].(string)
	clientKey := response.Data["clientkey"].(string)
	hmacSecret, err := wrappedClient.GetHmacSecret(walletKey)
	if err != nil {
		return nil, err
	}

	privateRequestSequences := make([]gemini.PrivateRequestSequence, 0)
	// sign each request
	for _, privateRequestRequirements := range *privateRequests {
		base := gemini.NewBulkPayoutPayload(
			clientID,
			&privateRequestRequirements,
		)
		signatures := []string{}
		// store the original nonce
		originalNonce := base.Nonce
		for i := 0; i < 10; i++ {
			// increment the nonce to correspond to each signature
			base.Nonce = originalNonce + int64(i)
			marshalled, err := json.Marshal(base)
			if err != nil {
				return nil, err
			}
			serializedPayload := base64.StdEncoding.EncodeToString(marshalled)
			sig, err := hmacSecret.HMACSha384(
				[]byte(serializedPayload),
			)
			if err != nil {
				return nil, err
			}
			signatures = append(signatures, hex.EncodeToString(sig))
		}
		base.Nonce = originalNonce
		requestSequence := gemini.PrivateRequestSequence{
			Signatures: signatures,
			Base:       base,
			APIKey:     clientKey,
		}
		privateRequestSequences = append(privateRequestSequences, requestSequence)
	}
	return &privateRequestSequences, nil
}

func createPaypalArtifact(
	ctx context.Context,
	outputFile string,
	client *vaultsigner.WrappedClient,
	walletKey string,
	paypalOnlySettlements []settlement.Transaction,
) error {
	return settlementcmd.PaypalTransformForMassPay(
		ctx,
		&paypalOnlySettlements,
		"JPY",
		decimal.NewFromFloat(viper.GetFloat64("jpyrate")),
		outputFile,
	)
}
