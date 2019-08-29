package main

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/brave-intl/bat-go/utils/formatters"
	"github.com/brave-intl/bat-go/utils/passphrase"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ed25519"
)

func main() {
	log.SetFormatter(&formatters.CliFormatter{})

	log.Println("Enter your recovery phrase:")
	reader := bufio.NewReader(os.Stdin)
	recoveryPhrase, err := reader.ReadString('\n')
	if err != nil {
		log.Fatalln(err)
	}

	seed, err := passphrase.ToBytes32(recoveryPhrase)
	if err != nil {
		log.Fatalln(err)
	}

	key, err := passphrase.DeriveSigningKeysFromSeed(seed, passphrase.LedgerHKDFSalt)
	if err != nil {
		log.Fatalln(err)
	}

	fmt.Println(hex.EncodeToString(key))
	fmt.Println(hex.EncodeToString(key.Public().(ed25519.PublicKey)))
}
