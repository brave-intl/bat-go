package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/brave-intl/bat-go/utils/vaultsigner"
)

var (
	privateKeyHex  = os.Getenv("ED25519_PRIVATE_KEY")
	publicKeyHex   = os.Getenv("ED25519_PUBLIC_KEY")
	geminiSecret   = os.Getenv("GEMINI_CLIENT_SECRET")
	geminiClientID = os.Getenv("GEMINI_CLIENT_ID")
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

	if len(privateKeyHex) != 0 && len(publicKeyHex) != 0 {
		fmt.Println("importing uphold key pair")
		// uphold importing
		err = upholdVaultImportKey(wrappedClient, "uphold-contribution")
		if err != nil {
			return err
		}
		err = upholdVaultImportKey(wrappedClient, "uphold-referral")
		if err != nil {
			return err
		}
	}
	if len(geminiSecret) != 0 {
		fmt.Println("importing gemini secret")
		geminiKVs := map[string]interface{}{
			"secret":    geminiSecret,
			"clientkey": geminiClientID,
		}
		// gemini importing
		err = geminiVaultImportValues(wrappedClient, "gemini-contribution", geminiKVs)
		if err != nil {
			return err
		}
		err = geminiVaultImportValues(wrappedClient, "gemini-referral", geminiKVs)
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
	return nil
}

func geminiVaultImportValues(
	wrappedClient *vaultsigner.WrappedClient,
	geminiImportName string,
	kvMap map[string]interface{},
) error {
	_, err := wrappedClient.ImportHmacSecret([]byte(geminiSecret), geminiImportName)
	if err != nil {
		return err
	}
	err = wrappedClient.GenerateMounts()
	if err != nil {
		return err
	}
	_, err = wrappedClient.Client.Logical().Write("wallets/"+geminiImportName, kvMap)
	return err
}
