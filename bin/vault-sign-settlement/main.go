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
	"github.com/brave-intl/bat-go/wallet"
	"github.com/brave-intl/bat-go/wallet/provider/uphold"
	"github.com/hashicorp/vault/api"
	util "github.com/hashicorp/vault/command/config"
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

	config := &api.Config{}
	err := config.ReadEnvironment()

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

	helper, err := util.DefaultTokenHelper()
	if err == nil {
		var token string
		token, err = helper.Get()
		if err == nil {
			client.SetToken(token)
		}
	}

	walletName := args[0]

	response, err := client.Logical().Read("wallets/" + walletName)
	if err != nil {
		log.Fatalln(err)
	}
	fmt.Println(response)

	providerId, ok := response.Data["providerId"]
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
	info.ProviderID = providerId.(string)
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

	err = settlement.PrepareTransactions(settlementWallet, settlements)
	if err != nil {
		log.Fatalln(err)
	}

	state := settlement.State{WalletInfo: settlementWallet.Info, Transactions: settlements}

	out, err := json.MarshalIndent(state, "", "    ")
	if err != nil {
		log.Fatalln(err)
	}

	err = ioutil.WriteFile(outputFile, out, 0400)
	if err != nil {
		log.Fatalln(err)
	}
}
