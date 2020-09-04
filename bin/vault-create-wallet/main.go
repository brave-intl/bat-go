package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/formatters"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	"github.com/brave-intl/bat-go/utils/vaultsigner"
	"github.com/brave-intl/bat-go/utils/wallet"
	"github.com/brave-intl/bat-go/utils/wallet/provider/uphold"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ed25519"
)

var flags = flag.NewFlagSet("", flag.ExitOnError)
var verbose = flags.Bool("v", false, "verbose output")
var offline = flags.Bool("offline", false, "operate in multi-step offline mode")

// State contains the current state of the registration
type State struct {
	WalletInfo   wallet.Info `json:"walletInfo"`
	Registration string      `json:"registration"`
}

func main() {
	log.SetFormatter(&formatters.CliFormatter{})

	flags.Usage = func() {
		log.Printf("Create a new wallet backed by vault.\n\n")
		log.Printf("Usage:\n\n")
		log.Printf("        %s WALLET_NAME\n\n", os.Args[0])
		log.Printf("  If a vault keypair exists with name WALLET_NAME, it will be used.\n")
		log.Printf("  Otherwise a new vault keypair with that name will be generated.\n\n")
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
		log.Printf("ERROR: Must pass a single argument to name generated wallet / keypair\n\n")
		flags.Usage()
		os.Exit(1)
	}

	name := args[0]
	logFile := name + "-registration.json"

	var state State
	var enc *json.Encoder

	if *offline {
		f, err := os.OpenFile(logFile, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0600)
		if err != nil {
			log.Fatalln(err)
		}

		dec := json.NewDecoder(f)

		for dec.More() {
			err := dec.Decode(&state)
			if err != nil {
				log.Fatalln(err)
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
			log.Fatalln(err)
		}

		signer, err := wrappedClient.GenerateEd25519Signer(name)
		if err != nil {
			log.Fatalln(err)
		}

		fmt.Printf("Keypair with public key: %s\n", signer)

		state.WalletInfo.PublicKey = signer.String()

		wallet := &uphold.Wallet{Info: state.WalletInfo, PrivKey: signer, PubKey: signer}

		reg, err := wallet.PrepareRegistration(name)
		if err != nil {
			log.Fatalln(err)
		}
		state.Registration = reg

		if *offline {
			err = enc.Encode(state)
			if err != nil {
				log.Fatalln(err)
			}

			fmt.Printf("Success, signed registration for wallet \"%s\"\n", name)
			fmt.Printf("Please copy %s to the online machine and re-run.\n", logFile)
			os.Exit(1)
		}
	}

	if len(state.WalletInfo.ProviderID) == 0 {
		var publicKey httpsignature.Ed25519PubKey
		publicKey, err := hex.DecodeString(state.WalletInfo.PublicKey)
		if err != nil {
			log.Fatalln(err)
		}
		wallet := uphold.Wallet{Info: state.WalletInfo, PrivKey: ed25519.PrivateKey{}, PubKey: publicKey}

		err = wallet.SubmitRegistration(state.Registration)
		if err != nil {
			log.Fatalln(err)
		}

		fmt.Printf("Success, registered new keypair and wallet \"%s\"\n", name)
		fmt.Printf("Uphold card ID %s\n", wallet.Info.ProviderID)
		state.WalletInfo.ProviderID = wallet.Info.ProviderID

		depositAddr, err := wallet.CreateCardAddress("ethereum")
		if err != nil {
			log.Fatalln(err)
		}
		fmt.Printf("ETH deposit addr: %s\n", depositAddr)

		if *offline {
			err = enc.Encode(state)
			if err != nil {
				log.Fatalln(err)
			}

			fmt.Printf("Please copy %s to the offline machine and re-run.\n", logFile)
			os.Exit(1)
		}
	}

	wrappedClient, err := vaultsigner.Connect()
	if err != nil {
		log.Fatalln(err)
	}

	err = wrappedClient.GenerateMounts()
	if err != nil {
		log.Fatalln(err)
	}

	_, err = wrappedClient.Client.Logical().Write("wallets/"+name, map[string]interface{}{
		"providerId": state.WalletInfo.ProviderID,
	})
	if err != nil {
		log.Fatalln(err)
	}

	fmt.Printf("Wallet setup complete!\n")
}
