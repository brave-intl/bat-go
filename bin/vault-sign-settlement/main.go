package main

import (
	"crypto"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/brave-intl/bat-go/cmd"
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
	var geminiOnlySettlements []settlement.Transaction
	for _, settlement := range settlements {
		if settlement.WalletProvider == "uphold" {
			upholdOnlySettlements = append(upholdOnlySettlements, settlement)
		} else if settlement.WalletProvider == "gemini" {
			geminiOnlySettlements = append(geminiOnlySettlements, settlement)
		}
	}
	if len(upholdOnlySettlements) > 0 {
		err = createUpholdArtifact(
			outputFile,
			settlementWallet,
			upholdOnlySettlements,
		)
		if err != nil {
			log.Fatalln(err)
		}
	}
	if len(geminiOnlySettlements) > 0 {
		err = createGeminiArtifact(
			outputFile,
			settlementWallet,
			geminiOnlySettlements,
		)
		if err != nil {
			log.Fatalln(err)
		}
	}
}

func createUpholdArtifact(
	outputFile string,
	settlementWallet *uphold.Wallet,
	upholdOnlySettlements []settlement.Transaction,
) error {
	err := settlement.PrepareTransactions(settlementWallet, upholdOnlySettlements)
	if err != nil {
		return err
	}

	state := settlement.State{WalletInfo: settlementWallet.Info, Transactions: upholdOnlySettlements}

	out, err := json.MarshalIndent(state, "", "    ")
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(outputFile, out, 0400)
	if err != nil {
		return err
	}
	return nil
}

func createGeminiArtifact(
	outputFile string,
	settlementWallet *uphold.Wallet,
	geminiOnlySettlements []settlement.Transaction,
) error {
	// group transactions (500 at a time)
	privateRequests, err := cmd.GeminiTransformTransactions(&geminiOnlySettlements)
	if err != nil {
		return err
	}
	// sign each request
	for _, privateRequestRequirements := range *privateRequests {
		sig, err := settlementWallet.PrivKey.Sign(
			rand.Reader,
			// base64 string
			[]byte(privateRequestRequirements.Payload),
			crypto.Hash(0),
		)
		if err != nil {
			return err
		}
		privateRequestRequirements.Signature = string(sig)
		privateRequestRequirements.APIKey = settlementWallet.PublicKey
	}
	// serialize requests to be sent in next step
	out, err := json.Marshal(privateRequests)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile("gemini-"+outputFile, out, 0400)
	if err != nil {
		return err
	}
	return nil
}
