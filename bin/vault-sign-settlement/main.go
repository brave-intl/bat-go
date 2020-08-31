package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/settlement"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/clients/gemini"
	"github.com/brave-intl/bat-go/utils/vaultsigner"
	"github.com/brave-intl/bat-go/utils/wallet"
	"github.com/brave-intl/bat-go/utils/wallet/provider/uphold"
	"github.com/shopspring/decimal"
)

var (
	inputFile = flag.String("in", "./contributions.json", "input file path")
	provider  = flag.String("provider", "", "wallet provider to settle out to")

	// split into provider / transaction type pairings
	allProviders []string
	// use correct vault key pair for each
	providerType = map[string][]string{
		"uphold": {"contribution", "referral"},
		"paypal": {"default"},
		"gemini": {"contribution", "referral"},
	}
	// providerByAntifraudInt = map[string]string{
	// 	"0": "uphold",
	// 	"1": "paypal",
	// 	"2": "gemini",
	// }
	artifactGenerators map[string]func(
		string,
		*vaultsigner.WrappedClient,
		string,
		[]settlement.Transaction,
	) error
)

func init() {
	// just add to providerType to add to allProviders
	allProviders = make([]string, 0, len(providerType))
	for k := range providerType {
		allProviders = append(allProviders, k)
	}
	// let the functions become initialized before creating the map
	artifactGenerators = map[string]func(
		string,
		*vaultsigner.WrappedClient,
		string,
		[]settlement.Transaction,
	) error{
		"uphold": createUpholdArtifact,
		"gemini": createGeminiArtifact,
		"paypal": createPaypalArtifact,
	}
}

func main() {
	log.SetFlags(0)

	flag.Usage = func() {
		log.Printf("Use a wallet backed by vault to sign settlements.\n\n")
		log.Printf("Usage:\n\n")
		log.Printf("        %s WALLET_NAME\n\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	// append -signed to the filename
	outputFile := strings.TrimSuffix(*inputFile, filepath.Ext(*inputFile)) + "-signed.json"

	// args := flag.Args()
	// if len(args) != 1 {
	// 	log.Printf("ERROR: Must pass a single argument, name of wallet / keypair\n\n")
	// 	flag.Usage()
	// 	os.Exit(1)
	// }

	// all settlements file
	settlementJSON, err := ioutil.ReadFile(*inputFile)
	if err != nil {
		log.Fatalln(err)
	}

	var antifraudSettlements []settlement.AntifraudTransaction
	err = json.Unmarshal(settlementJSON, &antifraudSettlements)
	if err != nil {
		log.Fatalln(err)
	}

	var providers []string
	if *provider == "" {
		// all providers
		providers = allProviders
	} else {
		providers = strings.Split(*provider, ",")
	}

	settlementsByProviderAndWalletKey := divideSettlementsByWallet(antifraudSettlements)

	wrappedClient, err := vaultsigner.Connect()
	if err != nil {
		log.Fatalln(err)
	}

	for _, provider := range providers {
		for _, walletSubDir := range providerType[provider] {
			walletKey := provider + "-" + walletSubDir
			settlements := settlementsByProviderAndWalletKey[walletKey]
			if len(settlements) == 0 {
				continue
			}
			err := artifactGenerators[provider](
				outputFile,
				wrappedClient,
				walletKey,
				settlements,
			)
			if err != nil {
				log.Fatalln(err)
			}
		}
	}
}

func divideSettlementsByWallet(antifraudTxs []settlement.AntifraudTransaction) map[string][]settlement.Transaction {
	settlementTransactionsByWallet := make(map[string][]settlement.Transaction)

	// alt := altcurrency.BAT
	for _, antifraudTx := range antifraudTxs {
		tx := antifraudTx.ToTransaction()

		provider := tx.WalletProvider
		wallet := tx.Type
		if provider == "paypal" {
			// might as well go into one (default)
			wallet = providerType[provider][0]
		}
		// which secret values to use to sign (paypal-default, uphold-referral, gemini-contribution)
		walletKey := provider + "-" + wallet
		// append to the nested structure
		settlementTransactionsByWallet[walletKey] = append(
			settlementTransactionsByWallet[walletKey],
			tx,
		)
	}
	return settlementTransactionsByWallet
}

func createUpholdArtifact(
	outputFile string,
	wrappedClient *vaultsigner.WrappedClient,
	walletKey string,
	upholdOnlySettlements []settlement.Transaction,
) error {
	response, err := wrappedClient.Client.Logical().Read("wallets/" + walletKey)
	if err != nil {
		return err
	}

	providerID, ok := response.Data["providerId"]
	if !ok {
		log.Fatalln("invalid wallet name")
	}

	signer, err := wrappedClient.GenerateEd25519Signer(walletKey)
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

	err = settlement.PrepareTransactions(settlementWallet, upholdOnlySettlements)
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

func getOauthClientID(
	wrappedClient *vaultsigner.WrappedClient,
	walletKey string,
) (string, error) {
	response, err := wrappedClient.Client.Logical().Read("wallets/" + walletKey)
	if err != nil {
		return "", err
	}
	oauthClientID, ok := response.Data["oauthClientId"]
	if !ok {
		return "", errors.New("oauth client id not set")
	}
	converted, ok := oauthClientID.(string)
	if !ok {
		return "", errors.New("unable to convert oauth client id to string")
	}
	return converted, nil
}

func createGeminiArtifact(
	outputFile string,
	wrappedClient *vaultsigner.WrappedClient,
	walletKey string,
	geminiOnlySettlements []settlement.Transaction,
) error {
	oauthClientID, err := getOauthClientID(wrappedClient, walletKey)
	if err != nil {
		return err
	}
	// group transactions (500 at a time)
	privateRequests, err := cmd.GeminiTransformTransactions(oauthClientID, geminiOnlySettlements)
	if err != nil {
		return err
	}
	privateRequests, err = signGeminiRequests(
		wrappedClient,
		walletKey,
		privateRequests,
	)
	if err != nil {
		return err
	}
	// serialize requests to be sent in next step
	out, err := json.Marshal(privateRequests)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile("gemini-"+outputFile, out, 0600)
	if err != nil {
		return err
	}
	return nil
}

func signGeminiRequests(
	wrappedClient *vaultsigner.WrappedClient,
	walletKey string,
	privateRequests *[]gemini.PrivateRequest,
) (*[]gemini.PrivateRequest, error) {
	response, err := wrappedClient.Client.Logical().Read("wallets/" + walletKey)
	if err != nil {
		return nil, err
	}

	// replace with new vault interface
	hmacSecret, err := wrappedClient.ImportHmacSecret(
		[]byte(response.Data["secret"].(string)),
		walletKey,
	)
	if err != nil {
		return nil, err
	}
	clientKey := response.Data["clientkey"].(string)

	// sign each request
	for i, privateRequestRequirements := range *privateRequests {
		sig, err := hmacSecret.Sign(
			// base64 string
			[]byte(privateRequestRequirements.Payload),
		)
		if err != nil {
			return nil, err
		}
		sigHex := hex.EncodeToString(sig)
		privateRequestRequirements.Signature = sigHex
		privateRequestRequirements.APIKey = clientKey
		(*privateRequests)[i] = privateRequestRequirements
	}
	return privateRequests, nil
}

func createPaypalArtifact(
	outputFile string,
	client *vaultsigner.WrappedClient,
	walletKey string,
	paypalOnlySettlements []settlement.Transaction,
) error {
	return cmd.PaypalTransformForMassPay(
		&paypalOnlySettlements,
		"JPY",
		decimal.NewFromFloat(0),
		outputFile,
	)
}
