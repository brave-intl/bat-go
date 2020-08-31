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
	privateKeyHex = os.Getenv("ED25519_PRIVATE_KEY")
	publicKeyHex  = os.Getenv("ED25519_PUBLIC_KEY")
	geminiSecret  = os.Getenv("GEMINI_CLIENT_SECRET")
)

func main() {
	err := validateAndImportSecrets()
	if err != nil {
		log.Fatalln(err)
	}
}

func validateAndImportSecrets() error {
	var err error
	if len(privateKeyHex) == 0 || len(publicKeyHex) == 0 {
		fmt.Println("importing uphold key pair")
		// uphold importing
		err = upholdVaultImportKey("wallets/uphold-contribution")
		if err != nil {
			return err
		}
		err = upholdVaultImportKey("wallets/uphold-referral")
		if err != nil {
			return err
		}
	}
	if len(geminiSecret) != 0 {
		fmt.Println("importing gemini secret")
		// gemini importing
		err = geminiVaultImportSecret("wallets/gemini-contribution")
		if err != nil {
			return err
		}
		err = geminiVaultImportSecret("wallets/gemini-referral")
		if err != nil {
			return err
		}
	}
	return nil
}

func upholdVaultImportKey(
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

	wrappedClient, err := vaultsigner.Connect()
	if err != nil {
		return err
	}

	_, err = wrappedClient.FromKeypair(privKey, pubKey, upholdImportName)
	if err != nil {
		return err
	}
	return nil
}

func geminiVaultImportSecret(
	geminiImportName string,
) error {
	wrappedClient, err := vaultsigner.Connect()
	if err != nil {
		return err
	}
	_, err = wrappedClient.ImportHmacSecret([]byte(geminiSecret), geminiImportName)
	if err != nil {
		return err
	}
	return nil
}
