package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/brave-intl/bat-go/settlement"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/vaultsigner"
	"github.com/brave-intl/bat-go/utils/wallet"
	"github.com/brave-intl/bat-go/utils/wallet/provider/uphold"
)

var (
	inputFile = flag.String("in", "./contributions.json", "input file path")
)

func main() {
	log.SetFlags(0)

	flag.Usage = func() {
		log.Printf("Use a wallet backed by vault to sign settlements.\n\n")
		log.Printf("Usage:\n\n")
		log.Printf("        %s WALLET_NAME\n\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	outputFile := strings.TrimSuffix(*inputFile, filepath.Ext(*inputFile)) + "-signed.json"

	args := flag.Args()
	if len(args) != 1 {
		log.Printf("ERROR: Must pass a single argument, name of wallet / keypair\n\n")
		flag.Usage()
		os.Exit(1)
	}

	client, err := vaultsigner.Connect()
	if err != nil {
		log.Fatalln(err)
	}

	walletName := args[0]

	response, err := client.Logical().Read("wallets/" + walletName)
	if err != nil {
		log.Fatalln(err)
	}
	fmt.Println(response)

	providerID, ok := response.Data["providerId"]
	if !ok {
		log.Fatalln("invalid wallet name")
	}

	signer, err := vaultsigner.New(client, walletName)
	if err != nil {
		log.Fatalln(err)
	}

	var info wallet.Info
	info.PublicKey = signer.String()
	info.Provider = "uphold"
	info.ProviderID = providerID.(string)
	{
		tmp := altcurrency.BAT
		info.AltCurrency = &tmp
	}
	settlementWallet := &uphold.Wallet{Info: info, PrivKey: signer, PubKey: signer}

	settlementJSON, err := ioutil.ReadFile(*inputFile)
	if err != nil {
		log.Fatalln(err)
	}

	var settlements []settlement.Transaction
	err = json.Unmarshal(settlementJSON, &settlements)
	if err != nil {
		log.Fatalln(err)
	}

	var upholdOnlySettlements []settlement.Transaction
	for i := 0; i < len(settlements); i++ {
		if settlements[i].WalletProvider == "uphold" {
			upholdOnlySettlements = append(upholdOnlySettlements, settlements[i])
		}
	}

	err = settlement.PrepareTransactions(settlementWallet, upholdOnlySettlements)
	if err != nil {
		log.Fatalln(err)
	}

	state := settlement.State{WalletInfo: settlementWallet.Info, Transactions: upholdOnlySettlements}

	out, err := json.MarshalIndent(state, "", "    ")
	if err != nil {
		log.Fatalln(err)
	}

	err = ioutil.WriteFile(outputFile, out, 0400)
	if err != nil {
		log.Fatalln(err)
	}
}
