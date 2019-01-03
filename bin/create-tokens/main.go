package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"time"

	"github.com/brave-intl/bat-go/grant"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/prompt"
	"github.com/brave-intl/bat-go/utils/vaultsigner"
	"github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"golang.org/x/crypto/ed25519"
	"gopkg.in/square/go-jose.v2"
	"gopkg.in/square/go-jose.v2/cryptosigner"
)

const (
	dateFormat        = "2006-01-02T15:04:05-0700"
	generated         = "(generated)"
	defaultValidWeeks = 24
)

var grantSignatorPrivateKeyHex = os.Getenv("GRANT_SIGNATOR_PRIVATE_KEY")

var flags = flag.NewFlagSet("", flag.ExitOnError)

var grantSigningKey = flags.String("grant-signing-key", "grant-signing-key", "name of vault transit key to use for signing")
var altCurrencyStr = flags.String("altcurrency", "BAT", "altcurrency for the grant [nominal unit for -value]")
var value = flags.Float64("value", 30.0, "value for the grant [nominal units, not probi]")
var numGrants = flags.Uint("num-grants", 50, "number of grants to create")
var maturityDateStr = flags.String("maturity-date", "now", "datetime when tokens should become redeemable [ISO 8601]")
var validWeeks = flags.Uint("valid-weeks", defaultValidWeeks, "weeks after the maturity date that tokens are valid before expiring [conflicts with -expiry-date]")
var expiryDateStr = flags.String("expiry-date", "", "datetime when tokens should expire [ISO 8601, conflicts with -valid-duration]")
var promotionID = flags.String("promotion-id", generated, "identifier for this promotion [uuidv4]")
var fromEnv = flags.Bool("env", false, "read private key from environment")
var outputFile = flags.String("out", "./grantTokens.json", "output file path")
var providerID = flags.String("provider-id", "", "bind this grant to a particular provider id [uuidv4]")
var grantType = flags.String("type", "ugp", "type for this grant [ugp|ads]")

type promotionInfo struct {
	ID                        uuid.UUID `json:"promotionId"`
	Priority                  int       `json:"priority"`
	Active                    bool      `json:"active"`
	MinimumReconcileTimestamp int64     `json:"minimumReconcileTimestamp"`
}

type grantRegistration struct {
	Grants     []string        `json:"grants"`
	Promotions []promotionInfo `json:"promotions"`
}

func newJoseVaultSigner(vSigner *vaultsigner.VaultSigner) (jose.Signer, error) {
	return jose.NewSigner(jose.SigningKey{Algorithm: jose.EdDSA, Key: cryptosigner.Opaque(vSigner)}, nil)
}

func main() {
	log.SetFlags(0)

	flags.Usage = func() {
		log.Printf("Create grant tokens, using vault held private keys or those passed via env vars.\n\n")
		log.Printf("Usage:\n\n")
		log.Printf("        %s [options]\n\n", os.Args[0])
		log.Printf("  Vault keypair exists with name grant-signing-key, it will be used (unless overridden).\n")
		log.Printf("  Otherwise a new vault keypair with that name will be generated.\n")
		log.Printf("  When -env is passed, key material is read from GRANT_SIGNATOR_PRIVATE_KEY and GRANT_SIGNATOR_PUBLIC_KEY\n\n")
		flags.PrintDefaults()
		log.Printf("\nExamples:\n\n")
		log.Printf("  Creating grant tokens for testing:\n\n")
		log.Printf("    1. Set the appropriate env vars: `GRANT_SIGNATOR_PRIVATE_KEY` and `GRANT_SIGNATOR_PUBLIC_KEY`\n")
		log.Printf("    2. Run the following command, adjusting the number of grants, expiry and maturity dates as needed\n\n")
		log.Printf("      ./create-tokens --env=true --num-grants=2000 --expiry-date=2022-05-08T00:00:00-0000 --maturity-date=2018-10-01T00:00:00-0000\n\n")
		log.Printf("    3. Run the following command to check your newly created tokens are tied to the correct key\n\n")
		log.Printf("      ./verify-tokens")
	}

	err := flags.Parse(os.Args[1:])
	if err != nil {
		log.Fatalln(err)
	}

	var altCurrency altcurrency.AltCurrency
	err = altCurrency.UnmarshalText([]byte(*altCurrencyStr))
	if err != nil {
		log.Fatalln(err)
	}

	if *value > 1000 {
		log.Fatalln("value is unreasonably large, did you accidentally provide probi?")
	}

	maturityDate := time.Now()
	if *maturityDateStr != "now" {
		maturityDate, err = time.Parse(dateFormat, *maturityDateStr)
		if err != nil {
			log.Fatalf("%s is not a valid ISO 8601 datetime\n", *maturityDateStr)
		}
	}

	if *validWeeks != defaultValidWeeks && len(*expiryDateStr) > 0 {
		log.Fatalln("Cannot pass both -expiry-date and -valid-duration")
	}

	var expiryDate time.Time
	if len(*expiryDateStr) > 0 {
		expiryDate, err = time.Parse(dateFormat, *expiryDateStr)
		if err != nil {
			log.Fatalf("%s is not a valid ISO 8601 datetime\n", *expiryDateStr)
		}
	} else {
		expiryDate = maturityDate.AddDate(0, 0, int(*validWeeks)*7)
	}

	promotionUUID := uuid.NewV4()
	if *promotionID != generated {
		promotionUUID, err = uuid.FromString(*promotionID)
		if err != nil {
			log.Fatalf("%s is not a valid uuidv4\n", *promotionID)
		}
	}

	var providerUUID *uuid.UUID
	if len(*providerID) > 0 {
		tmp, err := uuid.FromString(*providerID)
		if err != nil {
			log.Fatalf("%s is not a valid uuidv4\n", *providerID)
		}
		providerUUID = &tmp
	}

	var signer jose.Signer
	if *fromEnv {
		if len(grantSignatorPrivateKeyHex) == 0 {
			log.Fatalln("Must pass grant signing key via env var GRANT_SIGNATOR_PRIVATE_KEY")
		}

		var grantPrivateKey ed25519.PrivateKey
		grantPrivateKey, err = hex.DecodeString(grantSignatorPrivateKeyHex)
		if err != nil {
			log.Fatalln(err)
		}

		signer, err = jose.NewSigner(jose.SigningKey{Algorithm: "EdDSA", Key: grantPrivateKey}, nil)
		if err != nil {
			log.Fatalln(err)
		}
	} else {
		if len(grantSignatorPrivateKeyHex) > 0 {
			log.Fatalln("GRANT_SIGNATOR_PRIVATE_KEY should not be set when using vault key (missing --env flag)")
		}

		client, err := vaultsigner.Connect()
		if err != nil {
			log.Fatalln(err)
		}

		vSigner, err := vaultsigner.New(client, *grantSigningKey)
		if err != nil {
			log.Fatalln(err)
		}
		signer, err = newJoseVaultSigner(vSigner)
		if err != nil {
			log.Fatalln(err)
		}
	}

	fmt.Printf("Will create %d tokens worth %f %s each for promotion %s, valid starting on %s and expiring on %s\n", *numGrants, *value, altCurrency.String(), promotionUUID, maturityDate.String(), expiryDate.String())
	fmt.Print("Continue? ")
	resp, err := prompt.Bool()
	if err != nil {
		log.Fatalln(err)
	}
	if !resp {
		log.Fatalln("Exiting...")
	}

	grantTemplate := grant.Grant{
		AltCurrency:       &altCurrency,
		Probi:             altCurrency.ToProbi(decimal.NewFromFloat(*value)),
		PromotionID:       promotionUUID,
		MaturityTimestamp: maturityDate.Unix(),
		ExpiryTimestamp:   expiryDate.Unix(),
		Type:              *grantType,
		ProviderID:        providerUUID,
	}

	grants, err := grant.CreateGrants(signer, grantTemplate, *numGrants)
	if err != nil {
		log.Fatalln(err)
	}
	var grantReg grantRegistration
	grantReg.Grants = grants
	grantReg.Promotions = []promotionInfo{{ID: promotionUUID, Priority: 0, Active: false, MinimumReconcileTimestamp: maturityDate.Unix() * 1000}}
	serializedGrants, err := json.Marshal(grantReg)
	if err != nil {
		log.Fatalln(err)
	}
	err = ioutil.WriteFile(*outputFile, serializedGrants, 0600)
	if err != nil {
		log.Fatalln(err)
	}
}
