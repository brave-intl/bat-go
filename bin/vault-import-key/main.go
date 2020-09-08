package main

import (
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/brave-intl/bat-go/settlement"
	"github.com/brave-intl/bat-go/utils/vaultsigner"
)

var (
	privateKeyHex    = os.Getenv("ED25519_PRIVATE_KEY")
	publicKeyHex     = os.Getenv("ED25519_PUBLIC_KEY")
	upholdProviderID = os.Getenv("UPHOLD_PROVIDER_ID")
	geminiSecret     = os.Getenv("GEMINI_CLIENT_SECRET")
	geminiClientKey  = os.Getenv("GEMINI_CLIENT_KEY")

	configPath = flag.String("config", "", "read info from a config")
)

func main() {
	err := validateAndImportSecrets()
	if err != nil {
		log.Fatalln(err)
	}
}

func validateAndImportSecrets() error {
	var err error

	wrappedClient, err := vaultsigner.Connect()
	if err != nil {
		return err
	}

	config, err := settlement.ReadYamlConfig(*configPath)
	if err != nil {
		return err
	}

	if len(privateKeyHex) != 0 && len(publicKeyHex) != 0 {
		fmt.Println("importing uphold key pair")
		// uphold importing
		err = upholdVaultImportKey(wrappedClient, config.GetWalletKey("uphold-contribution"))
		if err != nil {
			return err
		}
		err = upholdVaultImportKey(wrappedClient, config.GetWalletKey("uphold-referral"))
		if err != nil {
			return err
		}
	}
	if len(geminiSecret) != 0 {
		fmt.Println("importing gemini secret")
		// gemini importing
		err = geminiVaultImportValues(wrappedClient, config.GetWalletKey("gemini-contribution"))
		if err != nil {
			return err
		}
		err = geminiVaultImportValues(wrappedClient, config.GetWalletKey("gemini-referral"))
		if err != nil {
			return err
		}
	}
	return nil
}

func upholdVaultImportKey(
	wrappedClient *vaultsigner.WrappedClient,
	upholdImportName string,
) error {
	privKey, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return errors.New("ERROR: Key material must be passed as hex")
	}

	pubKey, err := hex.DecodeString(publicKeyHex)
	if err != nil {
		return errors.New("ERROR: Key material must be passed as hex")
	}

	_, err = wrappedClient.FromKeypair(privKey, pubKey, upholdImportName)
	if err != nil {
		return err
	}

	_, err = wrappedClient.Client.Logical().Write("wallets/"+upholdImportName, map[string]interface{}{
		"providerId": upholdProviderID,
	})
	return err
}

func geminiVaultImportValues(
	wrappedClient *vaultsigner.WrappedClient,
	geminiImportName string,
) error {
	kvMap := map[string]interface{}{
		"clientkey": geminiClientKey,
	}
	_, err := wrappedClient.ImportHmacSecret([]byte(geminiSecret), geminiImportName)
	if err != nil {
		return err
	}
	_, err = wrappedClient.Client.Logical().Write("wallets/"+geminiImportName, kvMap)
	return err
}
