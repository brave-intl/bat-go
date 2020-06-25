package main

import (
	"bufio"
	"context"
	"flag"
	"os"

	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/formatters"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	"github.com/brave-intl/bat-go/utils/passphrase"
	"github.com/brave-intl/bat-go/utils/prompt"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/brave-intl/bat-go/wallet/provider"
	"github.com/brave-intl/bat-go/wallet/provider/uphold"
	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ed25519"
)

var currency = flag.String("currency", "BAT", "currency for transfer")
var from = flag.String("from", "", "identifier for the source wallet")
var note = flag.String("note", "", "optional note for the transfer")
var oneshot = flag.Bool("oneshot", false, "submit and commit in one shot")
var to = flag.String("to", "", "destination wallet address")
var value = flag.String("value", "", "amount to transfer [float or all]")
var verbose = flag.Bool("v", false, "verbose output")
var walletProvider = flag.String("provider", "uphold", "provider for the source wallet")

func main() {
	log.SetFormatter(&formatters.CliFormatter{})

	flag.Usage = func() {
		log.Printf("A utility for transferring funds.\n\n")
		log.Printf("Usage:\n\n")
		log.Printf("        %s -from SOURCE_WALLET_ID -to DEST_ADDRESS -value VALUE\n\n", os.Args[0])
		log.Printf("  %s signs and executes a one off transaction.\n\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	valueDec, err := decimal.NewFromString(*value)
	if *value != "all" && (err != nil || valueDec.LessThan(decimal.Zero)) {
		log.Printf("ERROR: Must pass -value greater than 0 or -value all\n\n")
		flag.Usage()
		os.Exit(1)
	}

	if len(*from) == 0 || len(*to) == 0 {
		log.Printf("ERROR: Must pass non-empty -from and -to\n\n")
		flag.Usage()
		os.Exit(1)
	}

	if *verbose {
		log.SetLevel(log.DebugLevel)
	}

	altc, err := altcurrency.FromString(*currency)
	if err != nil {
		log.Fatalln(err)
	}

	walletc := altcurrency.BAT
	info := wallet.Info{
		Provider:    *walletProvider,
		ProviderID:  *from,
		AltCurrency: &walletc,
	}
	w, err := provider.GetWallet(context.Background(), info)
	if err != nil {
		log.Fatalln(err)
	}

	var valueProbi decimal.Decimal
	var balance *wallet.Balance

	if walletc == altc {
		balance, err = w.GetBalance(true)
		if err != nil {
			log.Fatalln(err)
		}
	}

	if *value == "all" {
		if walletc == altc {
			valueProbi = balance.SpendableProbi
		} else {
			log.Fatalln("Sending all funds not available for currencies other than the wallet currency")
		}
	} else {
		valueProbi = altc.ToProbi(valueDec)
		if walletc == altc && balance.SpendableProbi.LessThan(valueProbi) {
			log.Fatalln("Insufficient funds in wallet")
		}
	}

	switch w := w.(type) {
	case *uphold.Wallet:
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

		w.PrivKey = key
		w.PubKey = httpsignature.Ed25519PubKey(key.Public().(ed25519.PublicKey))

		signedTx, err := w.PrepareTransaction(altc, valueProbi, *to, *note)
		if err != nil {
			log.Fatalln(err)
		}
		for {
			submitInfo, err := w.SubmitTransaction(signedTx, *oneshot)
			if err != nil {
				log.Fatalln(err)
			}
			if *oneshot {
				log.Println("Transfer complete.")
				break
			}

			log.Printf("Submitted quote for transfer, id: %s\n", submitInfo.ID)

			log.Printf("Will transfer %s %s from %s to %s\n", altc.FromProbi(valueProbi).String(), *currency, *from, *to)

			log.Printf("Continue? ")
			resp, err := prompt.Bool()
			if err != nil {
				log.Fatalln(err)
			}
			if !resp {
				log.Fatalln("Exiting...")
			}

			_, err = w.ConfirmTransaction(submitInfo.ID)
			if err != nil {
				log.Printf("error confirming: %s\n", err)
			}

			upholdInfo, err := w.GetTransaction(submitInfo.ID)
			if err != nil {
				log.Fatalln(err)
			}
			if upholdInfo.Status == "completed" {
				log.Println("Transfer complete.")
				break
			}

			log.Printf("Confirmation did not appear to go through, retry? ")
			resp, err = prompt.Bool()
			if err != nil {
				log.Fatalln(err)
			}
			if !resp {
				log.Fatalln("Exiting...")
			}
		}
	default:
		log.Fatalln("Unsupported wallet type")
	}
}
