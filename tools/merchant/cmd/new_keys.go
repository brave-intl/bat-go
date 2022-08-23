package merchant

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/brave-intl/bat-go/cmd"
	cmdutils "github.com/brave-intl/bat-go/cmd"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/cryptography"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

var (
	// MerchantCmd is a subcommand for macaroons
	MerchantCmd = &cobra.Command{
		Use:   "merchant",
		Short: "merchant subcommand",
	}
	// MerchantCreateKeyCmd generates a merchant attenuated key and id
	MerchantCreateKeyCmd = &cobra.Command{
		Use:   "create-key",
		Short: "create a merchant key",
		Run:   cmd.Perform("merchant key creation", RunMerchantCreateKey),
	}
)

func init() {
	MerchantCmd.AddCommand(
		MerchantCreateKeyCmd,
	)
	cmd.RootCmd.AddCommand(
		MerchantCmd,
	)

	createBuilder := cmdutils.NewFlagBuilder(MerchantCreateKeyCmd)

	createBuilder.Flag().String("encryption-key", "",
		"the environment encryption key").
		Bind("encryption-key").
		Env("ENCRYPTION_KEY")

	createBuilder.Flag().String("attenuation", "{}",
		"the attenuation for the created key").
		Bind("attenuation")
}

// RunMerchantCreateKey runs the generate command
func RunMerchantCreateKey(command *cobra.Command, args []string) error {
	encryptionKey, err := command.Flags().GetString("encryption-key")
	if err != nil {
		return err
	}
	if encryptionKey == "" {
		panic("no environment encryption key")
	}

	attenuation, err := command.Flags().GetString("attenuation")
	if err != nil {
		return err
	}
	if attenuation == "" {
		panic("no attenuation for key")
	}
	return CreateKey(
		command.Context(),
		encryptionKey,
		attenuation,
	)
}

// CreateKey - creates an attenuated merchant key
func CreateKey(ctx context.Context, encryptionKey, attenuation string) error {
	logger, lerr := appctx.GetLogger(ctx)
	if lerr != nil {
		_, logger = logging.SetupLogger(ctx)
	}

	var (
		merchantSecret string
		keyID          string = uuid.New().String()
		aKeyID         string
		aKeySecret     string
		err            error
	)

	// generate shared merchant secret
	merchantSecret, err = randomSecret(24)
	if err != nil {
		logger.Panic().
			Err(err).
			Msg("failed to generate merchant keys")
		return nil
	}
	// encrypt shared merchant secret
	var byteEncryptionKey [32]byte
	copy(byteEncryptionKey[:], []byte(encryptionKey)) // use env encrypt key

	encryptedBytes, n, err := cryptography.EncryptMessage(byteEncryptionKey, []byte(merchantSecret))
	if err != nil {
		logger.Panic().
			Err(err).
			Msg("failed to encrypt merchant keys")
		return nil
	}

	logger.Info().
		Str("keyID", fmt.Sprintf("%s", keyID)).
		Str("encryptedMerchantSecret", fmt.Sprintf("%x", encryptedBytes)).
		Str("encryptedMerchantNonce", fmt.Sprintf("%x", n)).
		Msg("encrypted secrets for insertion into database")

	// get the attenuation caveats
	var caveats = map[string]string{}
	err = json.Unmarshal([]byte(attenuation), &caveats)
	if err != nil {
		logger.Panic().
			Err(err).
			Msg("failed to decode attenuation caveats")
		return nil
	}

	aKeyID, aKeySecret, err = cryptography.Attenuate(keyID, merchantSecret, caveats)
	if err != nil {
		logger.Panic().
			Err(err).
			Msg("failed to attenuate merchant key")
		return nil
	}

	logger.Info().
		Str("merchantSecret", merchantSecret).
		Str("aKeyID", aKeyID).
		Str("aKeySecret", aKeySecret).
		Msg("attenuated merchant keys")

	return nil
}

func randomSecret(n int) (string, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	// Note that err == nil only if we read len(b) bytes.
	if err != nil {
		return "", err
	}

	return "secret-token:" + base64.RawURLEncoding.EncodeToString(b), nil
}
