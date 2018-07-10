package main

import (
	"encoding/hex"
	"flag"
	"log"
	"os"

	"github.com/brave-intl/bat-go/utils/vaultsigner"
)

var privateKeyHex = os.Getenv("ED25519_PRIVATE_KEY")
var publicKeyHex = os.Getenv("ED25519_PUBLIC_KEY")

func main() {
	log.SetFlags(0)

	flag.Usage = func() {
		log.Printf("A helper for importing existing ed25519 keys into vault.\n\n")
		log.Printf("Usage:\n\n")
		log.Printf("        %s VAULT_KEY_NAME\n\n", os.Args[0])
		log.Printf("  Hex key material is read from the environment, ED25519_PUBLIC_KEY and ED25519_PRIVATE_KEY.\n\n")
	}
	flag.Parse()

	if len(privateKeyHex) == 0 || len(publicKeyHex) == 0 {
		log.Printf("ERROR: Environment variables ED25519_PRIVATE_KEY and ED25519_PUBLIC_KEY must be passed\n\n")
		flag.Usage()
		os.Exit(1)
	}

	args := flag.Args()
	if len(args) != 1 {
		log.Printf("ERROR: Must pass a single argument to name imported keypair\n\n")
		flag.Usage()
		os.Exit(1)
	}

	privKey, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		log.Fatalln("ERROR: Key material must be passed as hex")
	}

	pubKey, err := hex.DecodeString(publicKeyHex)
	if err != nil {
		log.Fatalln("ERROR: Key material must be passed as hex")
	}

	client, err := vaultsigner.Connect()
	if err != nil {
		log.Fatalln(err)
	}

	_, err = vaultsigner.FromKeypair(client, privKey, pubKey, args[0])
	if err != nil {
		log.Fatalln(err)
	}
}
