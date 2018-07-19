package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"time"

	"github.com/brave-intl/bat-go/grant"
	"github.com/brave-intl/bat-go/utils"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/vaultsigner"
	"github.com/satori/go.uuid"
	"gopkg.in/square/go-jose.v2"
	"gopkg.in/square/go-jose.v2/cryptosigner"
)

const (
	dateFormat        = "2006-01-02T15:04:05-0700"
	generated         = "(generated)"
	defaultValidWeeks = 24
)

var grantSigningKey = flag.String("grant-signing-key", "grant-signing-key", "name of vault transit key to use for signing")
var altCurrencyStr = flag.String("altcurrency", "BAT", "altcurrency for the grant [nominal unit for -value]")
var value = flag.Float64("value", 30.0, "value for the grant [nominal units, not probi]")
var numGrants = flag.Uint("num-grants", 50, "number of grants to create")
var maturityDateStr = flag.String("maturity-date", "now", "datetime when tokens should become redeemable [ISO 8601]")
var validWeeks = flag.Uint("valid-weeks", defaultValidWeeks, "weeks after the maturity date that tokens are valid before expiring [conflicts with -expiry-date]")
var expiryDateStr = flag.String("expiry-date", "", "datetime when tokens should expire [ISO 8601, conflicts with -valid-duration]")
var promotionID = flag.String("promotion-id", generated, "identifier for this promotion [uuidv4]")
var outputFile = flag.String("out", "./grantTokens.json", "output file path")

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

func main() {
	log.SetFlags(0)

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

	client, err := vaultsigner.Connect()
	if err != nil {
		log.Fatalln(err)
	}

	vSigner, err := vaultsigner.New(client, *grantSigningKey)
	if err != nil {
		log.Fatalln(err)
	}
	cSigner := cryptosigner.Opaque(vSigner)
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: "EdDSA", Key: cSigner}, nil)
	if err != nil {
		log.Fatalln(err)
	}

	fmt.Printf("Will create %d tokens worth %f %s each for promotion %s, valid starting on %s and expiring on %s\n", *numGrants, *value, altCurrency.String(), promotionUUID, maturityDate.String(), expiryDate.String())
	fmt.Print("Continue? ")
	resp, err := utils.PromptBool()
	if err != nil {
		log.Fatalln(err)
	}
	if !resp {
		log.Fatalln("Exiting...")
	}

	grants := grant.CreateGrants(signer, promotionUUID, *numGrants, altCurrency, *value, maturityDate, expiryDate)
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
