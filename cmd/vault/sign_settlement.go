package vault

import (
	"context"
	"encoding/json"
	"errors"
	"io/ioutil"
	"path/filepath"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/cmd"
	settlementcmd "github.com/brave-intl/bat-go/cmd/settlement"
	"github.com/brave-intl/bat-go/settlement"
	geminisettlement "github.com/brave-intl/bat-go/settlement/gemini"
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
	SignSettlementCmd.Flags().String("in", "contributions.json",
		"input file path")
	cmd.Must(viper.BindPFlag("in", SignSettlementCmd.Flags().Lookup("in")))

	providers := []string{}
	for k := range providerTransactionTypes {
		providers = append(providers, k)
	}
	// providers -> the providers to parse out of the file and parse. default: uphold paypal gemini
	SignSettlementCmd.Flags().StringSlice("providers", providers,
		"providers to parse out of the given input files")
	cmd.Must(viper.BindPFlag("providers", SignSettlementCmd.Flags().Lookup("providers")))

	// jpyRate -> the providers to parse out of the file and parse. default: uphold paypal gemini
	SignSettlementCmd.Flags().Float64("jpyrate", 0.0,
		"jpyrate to use for paypal payouts")
	cmd.Must(viper.BindPFlag("jpyrate", SignSettlementCmd.Flags().Lookup("jpyrate")))
}

// SignSettlement runs the signing of a settlement
func SignSettlement(command *cobra.Command, args []string) error {
	ReadConfig(command)
	providers, err := command.Flags().GetStringSlice("providers")
	if err != nil {
		return err
	}
	inputFile, err := command.Flags().GetString("in")
	if err != nil {
		return err
	}
	// append -signed to the filename
	outputFile := strings.TrimSuffix(inputFile, filepath.Ext(inputFile)) + "-signed.json"
	logger, err := appctx.GetLogger(command.Context())
	if err != nil {
		return err
	}

	// all settlements file
	settlementJSON, err := ioutil.ReadFile(inputFile)
	if err != nil {
		return err
	}

	var antifraudSettlements []settlement.AntifraudTransaction
	err = json.Unmarshal(settlementJSON, &antifraudSettlements)
	if err != nil {
		return err
	}

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
			sublog := logger.With().
				Str("output", output).
				Str("provider", provider).
				Str("secretkey", secretKey).
				Int("settlements", len(settlements)).
				Logger()
			err := artifactGenerators[provider](
				sublog.WithContext(command.Context()),
				output,
				wrappedClient,
				secretKey,
				settlements,
			)
			if err != nil {
				return err
			}
			sublog.Info().Msg("created artifact")
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
		if !tx.Amount.GreaterThan(decimal.NewFromFloat(0)) {
			continue
		}
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
	privatePayloads, err := geminisettlement.TransformTransactions(ctx, oauthClientID, geminiOnlySettlements)
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
	return geminisettlement.SignRequests(
		clientID,
		clientKey,
		hmacSecret,
		privateRequests,
	)
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
