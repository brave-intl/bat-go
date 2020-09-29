package vault

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	"github.com/brave-intl/bat-go/utils/vaultsigner"
	"github.com/brave-intl/bat-go/utils/wallet"
	"github.com/brave-intl/bat-go/utils/wallet/provider/uphold"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/crypto/ed25519"
)

// State contains the current state of the registration
type State struct {
	WalletInfo   wallet.Info `json:"walletInfo"`
	Registration string      `json:"registration"`
}

var (
	// CreateWalletCmd transfer funds command
	CreateWalletCmd = &cobra.Command{
		Use:   "create-wallet",
		Short: "creates a wallet on a given provider",
		Run:   cmd.Perform("create wallet", CreateWallet),
	}
)

func init() {
	VaultCmd.AddCommand(
		CreateWalletCmd,
	)

	CreateWalletCmd.PersistentFlags().Bool("offline", false,
		"operate in multi-step offline mode")
	cmd.Must(viper.BindPFlag("offline", CreateWalletCmd.PersistentFlags().Lookup("offline")))
}

// CreateWallet creates a wallet
func CreateWallet(command *cobra.Command, args []string) error {
	offline := viper.GetBool("offline")
	logger, err := appctx.GetLogger(command.Context())
	if err != nil {
		return err
	}

	name := args[0]
	logFile := name + "-registration.json"

	var state State
	var enc *json.Encoder

	if offline {
		f, err := os.OpenFile(logFile, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0600)
		if err != nil {
			return err
		}

		dec := json.NewDecoder(f)

		for dec.More() {
			err := dec.Decode(&state)
			if err != nil {
				return err
			}
		}

		enc = json.NewEncoder(f)
	}

	if len(state.WalletInfo.PublicKey) == 0 || len(state.Registration) == 0 {
		var info wallet.Info
		info.Provider = "uphold"
		info.ProviderID = ""
		{
			tmp := altcurrency.BAT
			info.AltCurrency = &tmp
		}
		state.WalletInfo = info

		wrappedClient, err := vaultsigner.Connect()
		if err != nil {
			return err
		}

		signer, err := wrappedClient.GenerateEd25519Signer(name)
		if err != nil {
			return err
		}

		logger.Info().
			Str("provider", info.Provider).
			Str("public_key", signer.String()).
			Str("name", name).
			Msg("keypair")

		state.WalletInfo.PublicKey = signer.String()

		wallet := &uphold.Wallet{Info: state.WalletInfo, PrivKey: signer, PubKey: signer}

		reg, err := wallet.PrepareRegistration(name)
		if err != nil {
			return err
		}
		state.Registration = reg

		if offline {
			err = enc.Encode(state)
			if err != nil {
				return err
			}
			return fmt.Errorf("success, signed registration for wallet \"%s\"\nPlease copy %s to the online machine and re-run",
				name,
				logFile,
			)
		}
	}

	if len(state.WalletInfo.ProviderID) == 0 {
		var publicKey httpsignature.Ed25519PubKey
		publicKey, err := hex.DecodeString(state.WalletInfo.PublicKey)
		if err != nil {
			return err
		}
		wallet := uphold.Wallet{Info: state.WalletInfo, PrivKey: ed25519.PrivateKey{}, PubKey: publicKey}

		err = wallet.SubmitRegistration(state.Registration)
		if err != nil {
			return err
		}

		logger.Info().Msgf("Success, registered new keypair and wallet \"%s\"", name)
		logger.Info().Msgf("Uphold card ID %s", wallet.Info.ProviderID)
		state.WalletInfo.ProviderID = wallet.Info.ProviderID

		depositAddr, err := wallet.CreateCardAddress("ethereum")
		if err != nil {
			return err
		}
		logger.Info().Msgf("ETH deposit addr: %s", depositAddr)

		if offline {
			err = enc.Encode(state)
			if err != nil {
				return err
			}

			return fmt.Errorf("please copy %s to the offline machine and re-run", logFile)
		}
	}

	wrappedClient, err := vaultsigner.Connect()
	if err != nil {
		return err
	}

	err = wrappedClient.GenerateMounts()
	if err != nil {
		return err
	}

	_, err = wrappedClient.Client.Logical().Write("wallets/"+name, map[string]interface{}{
		"providerId": state.WalletInfo.ProviderID,
	})
	if err != nil {
		return err
	}

	logger.Info().Msg("Wallet setup complete!")
	return nil
}
