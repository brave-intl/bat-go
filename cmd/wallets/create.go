package wallets

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/brave-intl/bat-go/utils/wallet"
	"github.com/brave-intl/bat-go/utils/wallet/provider/uphold"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// CreateCmd creates a wallet on uphold
	CreateCmd = &cobra.Command{
		Use:   "create",
		Short: "creates a wallet",
		Run:   cmd.Perform("create", Create),
	}
)

func init() {
	WalletsCmd.AddCommand(CreateCmd)

	// name - the name of the new wallet
	CreateCmd.Flags().String("name", "",
		"the name for the wallet")
	cmd.Must(viper.BindPFlag("name", CreateCmd.Flags().Lookup("name")))
	CreateCmd.Flags().String("provider", "",
		"the provider for the wallet")
	cmd.Must(viper.BindPFlag("provider", CreateCmd.Flags().Lookup("provider")))
}

// Create creates a wallet
func Create(cmd *cobra.Command, args []string) error {
	provider, err := cmd.Flags().GetString("provider")
	fmt.Println("provider", err)
	if err != nil {
		return err
	}
	switch provider {
	case "uphold":
		name, err := cmd.Flags().GetString("name")
		if err != nil {
			return err
		}
		return CreateOnUphold(
			cmd.Context(),
			name,
		)
	}
	return nil
}

// CreateOnUphold creates a wallet on uphold
func CreateOnUphold(ctx context.Context, name string) error {
	logger, lerr := appctx.GetLogger(ctx)
	if lerr != nil {
		_, logger = logging.SetupLogger(ctx)
	}

	publicKey, privateKey, err := httpsignature.GenerateEd25519Key(nil)
	if err != nil {
		return err
	}
	publicKeyHex := hex.EncodeToString([]byte(publicKey))

	privateKeyHex := hex.EncodeToString([]byte(privateKey))
	logger.Info().
		Str("public_key", publicKeyHex).
		Str("private_key", privateKeyHex).
		Str("name", name).
		Msg("key created")

	var info wallet.Info
	info.Provider = "uphold"
	info.ProviderID = ""
	{
		tmp := altcurrency.BAT
		info.AltCurrency = &tmp
	}
	info.PublicKey = publicKeyHex

	wallet := &uphold.Wallet{Info: info, PrivKey: privateKey, PubKey: publicKey}

	err = wallet.Register(name)
	if err != nil {
		return err
	}

	logger.Info().
		Str("provider_id", wallet.Info.ProviderID).
		Msg("Uphold card ID")
	return nil
}
