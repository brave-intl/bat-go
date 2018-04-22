package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/brave-intl/bat-go/utils/vaultsigner"
	"github.com/hashicorp/vault/api"
)

var privateKeyHex = os.Getenv("ED25519_PRIVATE_KEY")
var publicKeyHex = os.Getenv("ED25519_PUBLIC_KEY")

func main() {
	log.SetFlags(0)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "A helper for importing existing ed25519 keys into vault.\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n\n")
		fmt.Fprintf(os.Stderr, "        %s VAULT_KEY_NAME\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  Hex key material is read from the environment, ED25519_PUBLIC_KEY and ED25519_PRIVATE_KEY.\n\n")
	}
	flag.Parse()

	if len(privateKeyHex) == 0 || len(publicKeyHex) == 0 {
		fmt.Fprintf(os.Stderr, "ERROR: Environment variables ED25519_PRIVATE_KEY and ED25519_PUBLIC_KEY must be passed\n\n")
		flag.Usage()
		os.Exit(1)
	}

	args := flag.Args()
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "ERROR: Must pass a single argument to name imported keypair\n\n")
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

	config := &api.Config{}
	err = config.ReadEnvironment()

	var client *api.Client
	if err != nil {
		client, err = api.NewClient(config)
	} else {
		client, err = api.NewClient(nil)
		if err != nil {
			log.Fatalln(err)
		}
		err = client.SetAddress("http://127.0.0.1:8200")
	}
	if err != nil {
		log.Fatalln(err)
	}

	_, err = vaultsigner.FromKeypair(client, privKey, pubKey, args[0])
	if err != nil {
		log.Fatalln(err)
	}
}
