package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/grant"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/satori/go.uuid"
	"github.com/square/go-jose"
	"golang.org/x/crypto/ed25519"
)

const (
	dateFormat        = "2006-01-02T15:04:05-0700"
	generated         = "(generated)"
	defaultValidWeeks = 24
)

var grantSignatorPrivateKeyHex = os.Getenv("GRANT_SIGNATOR_PRIVATE_KEY")

var altCurrencyStr = flag.String("altcurrency", "BAT", "altcurrency for the grant [nominal unit for -value]")
var value = flag.Uint("value", 30, "value for the grant [nominal units, not probi]")
var numGrants = flag.Uint("num-grants", 50, "number of grants to create")
var maturityDateStr = flag.String("maturity-date", "now", "datetime when tokens should become redeemable [ISO 8601]")
var validWeeks = flag.Uint("valid-weeks", defaultValidWeeks, "weeks after the maturity date that tokens are valid before expiring [conflicts with -expiry-date]")
var expiryDateStr = flag.String("expiry-date", "", "datetime when tokens should expire [ISO 8601, conflicts with -valid-duration]")
var promotionID = flag.String("promotion-id", generated, "identifier for this promotion [uuidv4]")
var outputFile = flag.String("out", "./grantTokens.json", "output file path")

type promotionInfo struct {
	ID       uuid.UUID `json:"promotionId"`
	Priority int       `json:"priority"`
	Active   bool      `json:"active"`
}

type grantRegistration struct {
	Grants     []string        `json:"grants"`
	Promotions []promotionInfo `json:"promotions"`
}

func main() {
	var err error
	flag.Parse()

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
			log.Fatalln("%s is not a valid ISO 8601 datetime", *expiryDateStr)
		}
	} else {
		expiryDate = maturityDate.AddDate(0, 0, int(*validWeeks))
	}

	promotionUUID := uuid.NewV4()
	if *promotionID != generated {
		promotionUUID, err = uuid.FromString(*promotionID)
		if err != nil {
			log.Fatalf("%s is not a valid uuidv4\n", *promotionID)
		}
	}

	if len(grantSignatorPrivateKeyHex) == 0 {
		log.Fatalln("Must pass grant signing key via env var GRANT_SIGNATOR_PRIVATE_KEY")
	}

	var grantPrivateKey ed25519.PrivateKey
	grantPrivateKey, err = hex.DecodeString(grantSignatorPrivateKeyHex)
	if err != nil {
		log.Fatalln(err)
	}

	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: "EdDSA", Key: grantPrivateKey}, nil)
	if err != nil {
		log.Fatalln(err)
	}

	fmt.Printf("Will create %d tokens worth %d %s each for promotion %s, valid starting on %s and expiring on %s\n", *numGrants, *value, altCurrency.String(), promotionUUID, maturityDate.String(), expiryDate.String())
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("Continue? (y/n): ")
		var text string
		text, err = reader.ReadString('\n')
		if err != nil {
			log.Fatalln(err)
		}
		if strings.ToLower(strings.TrimSpace(text)) == "n" {
			log.Fatalln("Exiting...")
		} else if strings.ToLower(strings.TrimSpace(text)) == "y" {
			break
		} else {
			fmt.Println("Input must be \"y\" or \"n\"")
		}
	}

	grants := grant.CreateGrants(signer, promotionUUID, *numGrants, altCurrency, *value, maturityDate, expiryDate)
	var grantReg grantRegistration
	grantReg.Grants = grants
	grantReg.Promotions = []promotionInfo{promotionInfo{promotionUUID, 0, false}}
	serializedGrants, err := json.Marshal(grantReg)
	if err != nil {
		log.Fatalln(err)
	}
	err = ioutil.WriteFile(*outputFile, serializedGrants, 0600)
	if err != nil {
		log.Fatalln(err)
	}
}
