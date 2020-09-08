package main

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	inputFile  = flag.String("in", "contributions.json", "input file path")
	provider   = flag.String("provider", "", "wallet provider to settle out to")
	jpyRate    = flag.Float64("rate", 0.0, "value of BAT in JPY")
	configPath = flag.String("config", "", "configuration file path")
	// split into provider / transaction type pairings
	allProviders []string
	// use correct vault key pair for each
	providerType = map[string][]string{
		"uphold": {"contribution", "referral"},
		"paypal": {"default"},
		"gemini": {"contribution", "referral"},
	}
	artifactGenerators map[string]func(
		string,
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

	config, err := settlement.ReadYamlConfig(*configPath)
	if err != nil {
		log.Fatalln(err)
	}

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
		for _, txType := range providerType[provider] {
			walletKey := provider + "-" + txType
			settlements := settlementsByProviderAndWalletKey[walletKey]
			if len(settlements) == 0 {
				continue
			}
			err := artifactGenerators[provider](
				txType,
				outputFile,
				wrappedClient,
				config.GetWalletKey(walletKey),
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
	txType string,
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
		return errors.New("invalid wallet name")
	}

	signer, err := wrappedClient.GenerateEd25519Signer(walletKey)
	if err != nil {
		return err
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

	err = ioutil.WriteFile("uphold-"+txType+"-"+outputFile, out, 0400)
	if err != nil {
		return err
	}
	return nil
}

func createGeminiArtifact(
	txType string,
	outputFile string,
	wrappedClient *vaultsigner.WrappedClient,
	walletKey string,
	geminiOnlySettlements []settlement.Transaction,
) error {
	// group transactions (500 at a time)
	privatePayloads, err := cmd.GeminiTransformTransactions(geminiOnlySettlements)
	if err != nil {
		return err
	}
	// leave enough space to increment nonce
	<-time.After(time.Microsecond)
	privateRequests, err := signGeminiRequests(
		wrappedClient,
		walletKey,
		privatePayloads,
	)
	if err != nil {
		return err
	}
	// serialize requests to be sent in next step
	out, err := json.Marshal(privateRequests)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile("gemini-"+txType+"-"+outputFile, out, 0400)
	if err != nil {
		return err
	}
	return nil
}

func signGeminiRequests(
	wrappedClient *vaultsigner.WrappedClient,
	walletKey string,
	privateRequests *[][]gemini.PayoutPayload,
) (*[]gemini.PrivateRequestSequence, error) {
	response, err := wrappedClient.Client.Logical().Read("wallets/" + walletKey)
	if err != nil {
		return nil, err
	}
	clientKey := response.Data["clientkey"].(string)
	hmacSecret, err := wrappedClient.GetHmacSecret(walletKey)
	if err != nil {
		return nil, err
	}

	privateRequestSequences := make([]gemini.PrivateRequestSequence, 0)
	// sign each request
	for _, privateRequestRequirements := range *privateRequests {
		base := gemini.NewBulkPayoutPayload(
			&privateRequestRequirements,
		)
		signatures := []string{}
		// store the original nonce
		originalNonce := base.Nonce
		for i := 0; i < 10; i++ {
			// increment the nonce to correspond to each signature
			base.Nonce = originalNonce + int64(i)
			marshalled, err := json.Marshal(base)
			if err != nil {
				return nil, err
			}
			serializedPayload := base64.StdEncoding.EncodeToString(marshalled)
			sig, err := hmacSecret.HMACSha384(
				[]byte(serializedPayload),
			)
			if err != nil {
				return nil, err
			}
			signatures = append(signatures, hex.EncodeToString(sig))
		}
		base.Nonce = originalNonce
		requestSequence := gemini.PrivateRequestSequence{
			Signatures: signatures,
			Base:       base,
			APIKey:     clientKey,
		}
		privateRequestSequences = append(privateRequestSequences, requestSequence)
	}
	return &privateRequestSequences, nil
}

func createPaypalArtifact(
	txType string,
	outputFile string,
	client *vaultsigner.WrappedClient,
	walletKey string,
	paypalOnlySettlements []settlement.Transaction,
) error {
	return cmd.PaypalTransformForMassPay(
		&paypalOnlySettlements,
		"JPY",
		decimal.NewFromFloat(*jpyRate),
		"paypal-"+outputFile,
	)
}
