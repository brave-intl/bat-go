package vault

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"

	rootcmd "github.com/brave-intl/bat-go/cmd"

	cmdutils "github.com/brave-intl/bat-go/cmd"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/httpsignature"
	vaultsigner "github.com/brave-intl/bat-go/tools/vault/signer"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	keysList = []string{
		"uphold-contribution",
		"uphold-referral",
		"gemini-contribution",
		"gemini-referral",
	}

	// ImportKeyCmd imports keys to be used in vault
	ImportKeyCmd = &cobra.Command{
		Use:   "import-key",
		Short: "import keys to be used in vault",
		Run:   rootcmd.Perform("import key", ImportKey),
	}
)

func init() {
	VaultCmd.AddCommand(
		ImportKeyCmd,
	)

	importKeyBuilder := cmdutils.NewFlagBuilder(ImportKeyCmd)

	// wallet-refs - default to keysList above. list of known keys
	// under which new wallet secrets can be referenced
	importKeyBuilder.Flag().StringSlice("wallet-refs", keysList,
		"the default path to a configuration file").
		Bind("wallet-refs")

	// config
	importKeyBuilder.Flag().String("config", "config.yaml",
		"config holds the mapping of wallet identifiers and secrets are to be held in vault").
		Bind("config")

	// ed25519-private-key
	importKeyBuilder.Flag().String("ed25519-private-key", "",
		"ed25519-private-key holds the private key in plaintext hex that we want to interact with").
		Bind("ed25519-private-key").
		Env("ED25519_PRIVATE_KEY")

	// ed25519-public-key
	importKeyBuilder.Flag().String("ed25519-public-key", "",
		"ed25519-public-key holds the public key in plaintext hex that we want to interact with").
		Bind("ed25519-public-key").
		Env("ED25519_PUBLIC_KEY")

	// uphold-provider-id
	importKeyBuilder.Flag().String("uphold-provider-id", "",
		"uphold-provider-id holds the uphold wallet guid that we want to interact with").
		Bind("uphold-provider-id").
		Env("UPHOLD_PROVIDER_ID")

	// gemini-client-id
	importKeyBuilder.Flag().String("gemini-client-id", "",
		"gemini-client-id holds the gemini oauth id used to pay transactions from a particular account").
		Bind("gemini-client-id").
		Env("GEMINI_CLIENT_ID")

	// gemini-client-key
	importKeyBuilder.Flag().String("gemini-client-key", "",
		"gemini-client-key holds the gemini key that is used by gemini to look up our hmac signing key").
		Bind("gemini-client-key").
		Env("GEMINI_CLIENT_KEY")

	// gemini-client-secret
	importKeyBuilder.Flag().String("gemini-client-secret", "",
		"gemini-client-secret holds the uphold guid that we want to use to sign bulk transactions").
		Bind("gemini-client-secret").
		Env("GEMINI_CLIENT_SECRET")
}

// ImportKey pulls in keys from environment variables
func ImportKey(command *cobra.Command, args []string) error {
	ReadConfig(command)
	walletRefs := viper.GetViper().GetStringSlice("wallet-refs")
	ed25519PrivateKey := viper.GetViper().GetString("ed25519-private-key")
	upholdProviderID := viper.GetViper().GetString("uphold-provider-id")
	geminiClientID := viper.GetViper().GetString("gemini-client-id")
	geminiClientKey := viper.GetViper().GetString("gemini-client-key")
	geminiClientSecret := viper.GetViper().GetString("gemini-client-secret")

	wrappedClient, err := vaultsigner.Connect()
	if err != nil {
		return err
	}

	for _, key := range walletRefs {
		parts := strings.Split(key, "-")
		switch parts[0] {
		case "uphold":
			if len(ed25519PrivateKey) != 0 {
				err = upholdVaultImportKey(
					command.Context(),
					wrappedClient,
					key,
					ed25519PrivateKey,
					upholdProviderID,
				)
				if err != nil {
					return err
				}
			}
		case "gemini":
			if len(geminiClientSecret) != 0 {
				err = geminiVaultImportValues(
					command.Context(),
					wrappedClient,
					key,
					geminiClientID,
					geminiClientKey,
					geminiClientSecret,
				)
				if err != nil {
					return err
				}
			}
		default:
			return errors.New("did not recognize option: " + key)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func upholdVaultImportKey(
	ctx context.Context,
	wrappedClient *vaultsigner.WrappedClient,
	key string,
	ed25519PrivateKey string,
	upholdProviderID string,
) error {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		return err
	}
	importName := Config.GetWalletKey(key)
	var privKey httpsignature.Ed25519PrivKey
	privKey, err = hex.DecodeString(ed25519PrivateKey)
	if err != nil {
		return errors.New("ERROR: Key material must be passed as hex")
	}

	if err := wrappedClient.GenerateMounts(); err != nil {
		return err
	}
	logger.Info().
		Str("provider", "uphold").
		Str("config-key", key).
		Str("vault-key", importName).
		Int("private-length", len(privKey)).
		Msg("importing secret")
	_, err = wrappedClient.FromKey(privKey, importName)
	if err != nil {
		return err
	}
	_, err = wrappedClient.Client.Logical().Write("wallets/"+importName, map[string]interface{}{
		"providerId": upholdProviderID,
	})
	return err
}

func geminiVaultImportValues(
	ctx context.Context,
	wrappedClient *vaultsigner.WrappedClient,
	key string,
	geminiClientID string,
	geminiClientKey string,
	geminiClientSecret string,
) error {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		return err
	}
	importName := Config.GetWalletKey(key)
	if err := wrappedClient.GenerateMounts(); err != nil {
		return err
	}
	logger.Info().
		Str("provider", "gemini").
		Str("config-key", key).
		Str("vault-key", importName).
		Int("secret-length", len(geminiClientSecret)).
		Msg("importing secret")
	_, err = wrappedClient.ImportHmacSecret([]byte(geminiClientSecret), importName)
	if err != nil {
		return err
	}
	_, err = wrappedClient.Client.Logical().Write("wallets/"+importName, map[string]interface{}{
		"clientid":  geminiClientID,
		"clientkey": geminiClientKey,
	})
	return err
}
