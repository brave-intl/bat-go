package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/formatters"
	"github.com/brave-intl/bat-go/utils/prompt"
	"github.com/brave-intl/bat-go/utils/vaultsigner"
	"github.com/brave-intl/bat-go/utils/wallet"
	"github.com/brave-intl/bat-go/utils/wallet/provider/uphold"
	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
)

var currency = flag.String("currency", "BAT", "currency for transfer")
var from = flag.String("from", "", "vault name for the source wallet")
var note = flag.String("note", "", "optional note for the transfer")
var oneshot = flag.Bool("oneshot", false, "submit and commit in one shot")
var to = flag.String("to", "", "destination wallet address")
var value = flag.String("value", "", "amount to transfer [float or all]")
var verbose = flag.Bool("v", false, "verbose output")

func main() {
	log.SetFormatter(&formatters.CliFormatter{})

	flag.Usage = func() {
		log.Printf("A utility for transferring funds.\n\n")
		log.Printf("Usage:\n\n")
		log.Printf("        %s -from SOURCE_WALLET_NAME -to DEST_ADDRESS -value VALUE\n\n", os.Args[0])
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

	wrappedClient, err := vaultsigner.Connect()
	if err != nil {
		log.Fatalln(err)
	}

	response, err := wrappedClient.Client.Logical().Read("wallets/" + *from)
	if err != nil {
		log.Fatalln(err)
	}
	fmt.Println(response)

	providerID, ok := response.Data["providerId"]
	if !ok {
		log.Fatalln("invalid wallet name")
	}

	signer, err := wrappedClient.GenerateEd25519Signer(*from)
	if err != nil {
		log.Fatalln(err)
	}

	walletc := altcurrency.BAT

	var info wallet.Info
	info.PublicKey = signer.String()
	info.Provider = "uphold"
	info.ProviderID = providerID.(string)
	{
		tmp := walletc
		info.AltCurrency = &tmp
	}

	w := &uphold.Wallet{Info: info, PrivKey: signer, PubKey: signer}

	altc, err := altcurrency.FromString(*currency)
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
}
