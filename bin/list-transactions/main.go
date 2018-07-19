package main

import (
	"flag"
	"os"

	"github.com/brave-intl/bat-go/utils"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/brave-intl/bat-go/wallet/provider"
	log "github.com/sirupsen/logrus"
)

var verbose = flag.Bool("v", false, "verbose output")
var walletProvider = flag.String("provider", "uphold", "provider for the source wallet")

func main() {
	log.SetFormatter(&utils.CliFormatter{})

	flag.Usage = func() {
		log.Printf("A helper for fetching transaction history.\n\n")
		log.Printf("Usage:\n\n")
		log.Printf("        %s PROVIDER_ID\n", os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse()

	if *verbose {
		log.SetLevel(log.DebugLevel)
	}

	if len(flag.Args()) != 1 {
		flag.Usage()
		os.Exit(1)
	}

	walletc := altcurrency.BAT
	info := wallet.Info{
		Provider:    *walletProvider,
		ProviderID:  flag.Args()[0],
		AltCurrency: &walletc,
	}
	w, err := provider.GetWallet(info)
	if err != nil {
		log.Fatalln(err)
	}

	txns, err := w.ListTransactions()
	if err != nil {
		log.Fatalln(err)
	}

	for i := 0; i < len(txns); i++ {
		log.Printf("%s\n", txns[i])
	}
}
