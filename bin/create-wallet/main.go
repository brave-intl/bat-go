package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"os"

	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/formatters"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/brave-intl/bat-go/wallet/provider/uphold"
	log "github.com/sirupsen/logrus"
)

var flags = flag.NewFlagSet("", flag.ExitOnError)
var verbose = flags.Bool("v", false, "verbose output")

func main() {
	log.SetFormatter(&formatters.CliFormatter{})

	flags.Usage = func() {
		log.Printf("Create a new wallet.\n\n")
		log.Printf("Usage:\n\n")
		log.Printf("        %s WALLET_NAME\n\n", os.Args[0])
		flags.PrintDefaults()
	}
	err := flags.Parse(os.Args[1:])
	if err != nil {
		log.Fatalln(err)
	}

	if *verbose {
		log.SetLevel(log.DebugLevel)
	}

	args := flags.Args()
	if len(args) != 1 {
		log.Printf("ERROR: Must pass a single argument to name generated wallet\n\n")
		flags.Usage()
		os.Exit(1)
	}

	name := args[0]

	publicKey, privateKey, err := httpsignature.GenerateEd25519Key(nil)
	if err != nil {
		log.Fatalln(err)
	}
	publicKeyHex := hex.EncodeToString([]byte(publicKey))
	fmt.Println("publicKey: ", publicKeyHex)

	privateKeyHex := hex.EncodeToString([]byte(privateKey))
	fmt.Println("privateKey: ", privateKeyHex)

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
		log.Fatalln(err)
	}

	fmt.Printf("Uphold card ID %s\n", wallet.Info.ProviderID)
}
