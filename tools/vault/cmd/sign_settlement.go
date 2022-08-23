package vault

import (
	"context"
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	rootcmd "github.com/brave-intl/bat-go/cmd"

	cmdutils "github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/libs/altcurrency"
	"github.com/brave-intl/bat-go/libs/clients/bitflyer"
	"github.com/brave-intl/bat-go/libs/clients/gemini"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/custodian"
	"github.com/brave-intl/bat-go/libs/wallet"
	"github.com/brave-intl/bat-go/libs/wallet/provider/uphold"
	settlement "github.com/brave-intl/bat-go/tools/settlement"
	bitflyersettlement "github.com/brave-intl/bat-go/tools/settlement/bitflyer"
	settlementcmd "github.com/brave-intl/bat-go/tools/settlement/cmd"
	geminisettlement "github.com/brave-intl/bat-go/tools/settlement/gemini"
	upholdsettlement "github.com/brave-intl/bat-go/tools/settlement/uphold"
	vaultsigner "github.com/brave-intl/bat-go/tools/vault/signer"
	"github.com/shopspring/decimal"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// SignSettlementCmd signs a settlement file's transactions
	SignSettlementCmd = &cobra.Command{
		Use:   "sign-settlement INPUT_FILE...",
		Short: "sign settlement files using vault held secrets",
		Run:   rootcmd.Perform("sign settlement", SignSettlement),
	}
	// the combination of provider + transaction type gives you the key
	// under which the vault secrets are located by default
	providerTransactionTypes = map[string][]string{
		"uphold":   {"contribution", "referral", "adsDirectDeposit"},
		"paypal":   {"default"},
		"gemini":   {"contribution", "referral", "adsDirectDeposit"},
		"bitflyer": {"default"},
	}
	artifactGenerators = map[string]func(
		context.Context,
		string,
		*vaultsigner.WrappedClient,
		string,
		[]custodian.Transaction,
	) error{
		"uphold":   createUpholdArtifact,
		"gemini":   createGeminiArtifact,
		"paypal":   createPaypalArtifact,
		"bitflyer": createBitflyerArtifact,
	}
)

func init() {
	VaultCmd.AddCommand(
		SignSettlementCmd,
	)

	signSettlementBuilder := cmdutils.NewFlagBuilder(SignSettlementCmd)

	// in -> the file to parse and sign according to each provider's setup. default: contributions.json
	signSettlementBuilder.Flag().String("in", "contributions.json",
		"( legacy compatibility ) input file path").
		Bind("in")

	signSettlementBuilder.Flag().BoolP("merge-custodial", "m", false, "If present, combine multiple addresses which share a walletProviderID into a single transaction.")

	providers := []string{}
	for k := range providerTransactionTypes {
		providers = append(providers, k)
	}
	// providers -> the providers to parse out of the file and parse. default: uphold paypal gemini
	signSettlementBuilder.Flag().StringSlice("providers", providers,
		"providers to parse out of the given input files").
		Bind("providers")

	// jpyRate -> the providers to parse out of the file and parse. default: uphold paypal gemini
	signSettlementBuilder.Flag().Float64("jpyrate", 0.0,
		"jpyrate to use for paypal payouts").
		Bind("jpyrate")

	signSettlementBuilder.Flag().String("config", "config.yaml",
		"the default path to a configuration file").
		Bind("config").
		Env("CONFIG")

	signSettlementBuilder.Flag().String("out", "",
		"the output directory for prepared files ( defaults to current working directory )").
		Bind("out")

	signSettlementBuilder.Flag().Bool("merge", false,
		"when multiple input files are passed, merge all transactions and output one file per provider / transaction type ").
		Bind("merge")

	signSettlementBuilder.Flag().String("bitflyer-client-token", "",
		"the token to be sent for auth on bitflyer").
		Bind("bitflyer-client-token").
		Env("BITFLYER_TOKEN")

	signSettlementBuilder.Flag().String("bitflyer-client-id", "",
		"tells bitflyer what the client id is during token generation").
		Bind("bitflyer-client-id").
		Env("BITFLYER_CLIENT_ID")

	signSettlementBuilder.Flag().String("bitflyer-client-secret", "",
		"tells bitflyer what the client secret during token generation").
		Bind("bitflyer-client-secret").
		Env("BITFLYER_CLIENT_SECRET")

	signSettlementBuilder.Flag().Bool("exclude-limited", false,
		"in order to avoid not knowing what the payout amount will be because of transfer limits").
		Bind("exclude-limited")

	signSettlementBuilder.Flag().Int("chunk-size", 0,
		"how many transfers to combine per request, 0 indicates the default value").
		Bind("chunk-size")
}

// SignSettlement runs the signing of a settlement
func SignSettlement(command *cobra.Command, args []string) error {
	inputFiles := args

	ReadConfig(command)
	providers, err := command.Flags().GetStringSlice("providers")
	if err != nil {
		return err
	}
	inputFile, err := command.Flags().GetString("in")
	if err != nil {
		return err
	}
	if len(inputFiles) == 0 {
		inputFiles = []string{inputFile}
	}
	outDir, err := command.Flags().GetString("out")
	if err != nil {
		return err
	}
	merge, err := command.Flags().GetBool("merge")
	if err != nil {
		return err
	}
	mergeCustodial, err := command.Flags().GetBool("merge-custodial")
	if err != nil {
		return err
	}

	logger, err := appctx.GetLogger(command.Context())
	if err != nil {
		return err
	}

	if len(outDir) > 0 {
		logger.Info().Str("outDir", outDir).Msg("creating output directory")

		err := os.MkdirAll(outDir, os.ModePerm)
		if err != nil {
			return err
		}
	}

	var mergedSettlements []settlement.AntifraudTransaction

	for _, inputFile := range inputFiles {
		sublog := logger.With().Str("inputFile", inputFile).Logger()

		sublog.Info().Msg("reading settlement file")

		// append -signed to the filename
		outBaseFile := strings.TrimSuffix(filepath.Base(inputFile), filepath.Ext(inputFile)) + "-signed.json"

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

		sublog.Info().Int("len(antifraudSettlements)", len(antifraudSettlements)).Msg("deserialized settlement file")

		mergedSettlements = append(mergedSettlements, antifraudSettlements...)
		if merge {
			sublog.Info().Int("len(mergedSettlements)", len(mergedSettlements)).Msg("merged settlements")
		} else {
			return processSettlements(
				sublog.WithContext(
					context.WithValue(
						command.Context(),
						appctx.MergeCustodialCTXKey,
						mergeCustodial,
					),
				),
				providers,
				outDir,
				outBaseFile,
				antifraudSettlements,
			)
		}
	}

	err = settlement.CheckForDuplicates(mergedSettlements)
	if err != nil {
		return err
	}

	if merge {
		logger.Info().Int("len(mergedSettlements)", len(mergedSettlements)).Msg("processing merged settlements")
		return processSettlements(command.Context(), providers, outDir, "merged-signed.json", mergedSettlements)
	}
	return nil
}

func processSettlements(ctx context.Context, providers []string, outDir string, outBaseFile string, antifraudSettlements []settlement.AntifraudTransaction) error {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		return err
	}

	settlementsByProviderAndWalletKey, err := divideSettlementsByWallet(ctx, antifraudSettlements)
	if err != nil {
		return err
	}

	logLine := logger.Info()
	for _, provider := range providers {
		for _, txType := range providerTransactionTypes[provider] {
			walletKey := provider + "-" + txType
			logLine = logLine.Int(walletKey, len(settlementsByProviderAndWalletKey[walletKey]))
		}
	}
	logLine.Msg("split settlements by provider and transaction type")

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
			output := filepath.Join(outDir, walletKey+"-"+outBaseFile)

			secretKey := Config.GetWalletKey(walletKey)

			sublog := logger.With().
				Str("provider", provider).
				Str("txType", txType).
				Str("output", output).
				Int("settlements", len(settlements)).
				Logger()

			sublog.Info().Str("wallet", secretKey).Msg("attempting to sign settlements")

			err := artifactGenerators[provider](
				sublog.WithContext(ctx),
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

func divideSettlementsByWallet(ctx context.Context, antifraudTxs []settlement.AntifraudTransaction) (map[string][]custodian.Transaction, error) {
	settlementTransactionsByWallet := make(map[string][]custodian.Transaction)

	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		return settlementTransactionsByWallet, err
	}

	for _, antifraudTx := range antifraudTxs {
		tx, err := antifraudTx.ToTransaction()
		if err != nil {
			logger.Warn().Err(err).Str("channel", tx.Channel).Msg("skipping transaction as it failed to validate")
			continue
		}

		provider := tx.WalletProvider
		wallet := tx.Type
		if len(providerTransactionTypes[provider]) == 1 {
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
	return settlementTransactionsByWallet, nil
}

func createUpholdArtifact(
	ctx context.Context,
	outputFile string,
	wrappedClient *vaultsigner.WrappedClient,
	walletKey string,
	upholdOnlySettlements []custodian.Transaction,
) error {
	var transactionSet []custodian.Transaction = upholdOnlySettlements
	response, err := wrappedClient.Client.Logical().Read("wallets/" + walletKey)
	if err != nil {
		return err
	}

	providerID, ok := response.Data["providerId"]
	if !ok {
		return errors.New("invalid wallet name")
	}

	// Ensure that there is only one payment for a given Uphold wallet by combining
	// multiples if they exist.
	mergeCustodialRaw := ctx.Value(appctx.MergeCustodialCTXKey)
	mergeCustodial, ok := mergeCustodialRaw.(bool)
	if ok && mergeCustodial == true {
		transactionSet = upholdsettlement.FlattenPaymentsByWalletProviderID(
			&upholdOnlySettlements,
		)
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

	err = settlement.PrepareTransactions(settlementWallet, transactionSet, "payout", &uphold.Beneficiary{Relationship: "business"})
	if err != nil {
		return err
	}

	state := settlement.State{WalletInfo: settlementWallet.Info, Transactions: transactionSet}

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

// NewRefreshTokenPayloadFromViper creates the payload to refresh a bitflyer token from viper
func NewRefreshTokenPayloadFromViper() *bitflyer.TokenPayload {
	vpr := viper.GetViper()
	clientID := vpr.GetString("bitflyer-client-id")
	clientSecret := vpr.GetString("bitflyer-client-secret")
	extraClientSecret := vpr.GetString("bitflyer-extra-client-secret")
	return &bitflyer.TokenPayload{
		GrantType:         "client_credentials",
		ClientID:          clientID,
		ClientSecret:      clientSecret,
		ExtraClientSecret: extraClientSecret,
	}
}

func createBitflyerArtifact(
	ctx context.Context,
	outputFile string,
	wrappedClient *vaultsigner.WrappedClient,
	walletKey string,
	bitflyerOnlySettlements []custodian.Transaction,
) error {
	bitflyerClient, err := bitflyer.New()
	if err != nil {
		return err
	}
	refreshTokenPayload := NewRefreshTokenPayloadFromViper()
	_, err = bitflyerClient.RefreshToken(ctx, *refreshTokenPayload)
	if err != nil {
		return err
	}

	vpr := viper.GetViper()
	exclude := vpr.GetBool("exclude-limited")
	sourceFrom := vpr.GetString("bitflyer-source-from")

	preparedTransactions, err := bitflyersettlement.PrepareRequests(
		ctx,
		bitflyerClient,
		bitflyerOnlySettlements,
		exclude,
		sourceFrom,
	)
	if err != nil {
		return err
	}
	out, err := json.MarshalIndent(preparedTransactions, "", "  ")
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
	geminiOnlySettlements []custodian.Transaction,
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
	out, err := json.MarshalIndent(privateRequests, "", "  ")
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
	paypalOnlySettlements []custodian.Transaction,
) error {
	return settlementcmd.PaypalTransformForMassPay(
		ctx,
		&paypalOnlySettlements,
		"JPY",
		decimal.NewFromFloat(viper.GetFloat64("jpyrate")),
		outputFile,
	)
}
