package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/grant"
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

// TokenContext the data to be used to create tokens
type TokenContext struct {
	AltCurrency   altcurrency.AltCurrency
	MaturityDate  time.Time
	ExpiryDate    time.Time
	PromotionUUID uuid.UUID
}

func main() {
	log.SetFlags(0)
	flag.Parse()
	context, err := BuildContext()
	if err != nil {
		log.Fatalln(err)
	}
	signer, err := CreateSigner()
	if err != nil {
		log.Fatalln(err)
	}

	accepted, err := ReceiveInput(context)
	if err != nil {
		log.Fatalln(err)
	}
	if !accepted {
		log.Fatalln("rejected creation")
	}

	err = CreateTokens(signer, context)
	if err != nil {
		log.Fatalln(err)
	}
}

// BuildContext creates a structure for other functions to referrence
func BuildContext() (context TokenContext, err error) {
	var altCurrency altcurrency.AltCurrency
	err = altCurrency.UnmarshalText([]byte(*altCurrencyStr))
	if err != nil {
		return context, err
	}

	if *value > 1000 {
		return context, errors.New("value is unreasonably large, did you accidentally provide probi?")
	}

	maturityDate := time.Now()
	if *maturityDateStr != "now" {
		maturityDate, err = time.Parse(dateFormat, *maturityDateStr)
		if err != nil {
			return context, fmt.Errorf("%s is not a valid ISO 8601 datetime", *maturityDateStr)
		}
	}

	if *validWeeks != defaultValidWeeks && len(*expiryDateStr) > 0 {
		return context, errors.New("Cannot pass both -expiry-date and -valid-duration")
	}

	var expiryDate time.Time
	if len(*expiryDateStr) > 0 {
		expiryDate, err = time.Parse(dateFormat, *expiryDateStr)
		if err != nil {
			return context, fmt.Errorf("%s is not a valid ISO 8601 datetime", *expiryDateStr)
		}
	} else {
		expiryDate = maturityDate.AddDate(0, 0, int(*validWeeks)*7)
	}

	promotionUUID := uuid.NewV4()
	if *promotionID != generated {
		promotionUUID, err = uuid.FromString(*promotionID)
		if err != nil {
			return context, fmt.Errorf("%s is not a valid uuidv4", *promotionID)
		}
	}
	context = TokenContext{
		AltCurrency:   altCurrency,
		MaturityDate:  maturityDate,
		ExpiryDate:    expiryDate,
		PromotionUUID: promotionUUID,
	}
	return context, nil
}

// CreateSigner creates a signer
func CreateSigner() (signer jose.Signer, err error) {
	client, err := vaultsigner.Connect()
	if err != nil {
		return nil, err
	}

	vSigner, err := vaultsigner.New(client, *grantSigningKey)
	if err != nil {
		return nil, err
	}

	cSigner := cryptosigner.Opaque(vSigner)
	signingKey := jose.SigningKey{Algorithm: jose.EdDSA, Key: cSigner}
	signer, err = jose.NewSigner(signingKey, nil)
	if err != nil {
		return nil, err
	}
	return signer, nil
}

// ReceiveInput takes in stdin from the buffer reader
func ReceiveInput(context TokenContext) (result bool, err error) {
	promotionUUID := context.PromotionUUID
	altCurrency := context.AltCurrency
	maturityDate := context.MaturityDate
	expiryDate := context.ExpiryDate
	fmt.Printf("Will create %d tokens worth %f %s each for promotion %s, valid starting on %s and expiring on %s\n", *numGrants, *value, altCurrency.String(), promotionUUID, maturityDate.String(), expiryDate.String())
	reader := bufio.NewReader(os.Stdin)
	var text string
	for {
		fmt.Print("Continue? (y/n): ")
		text, err = reader.ReadString('\n')
		if err != nil {
			return result, err
		}
		accepted, err := CheckInput(text)
		if err == nil {
			return accepted, nil
		}
		fmt.Println("Input must be \"y\" or \"n\"")
	}
}

// CheckInput checks the input and gives back
func CheckInput(text string) (bool, error) {
	if strings.ToLower(strings.TrimSpace(text)) == "n" {
		return false, nil
	} else if strings.ToLower(strings.TrimSpace(text)) == "y" {
		return true, nil
	}
	return false, errors.New("did not match")
}

// CreateTokens creates tokens from the signer and context object
func CreateTokens(signer jose.Signer, context TokenContext) error {
	promotionUUID := context.PromotionUUID
	altCurrency := context.AltCurrency
	maturityDate := context.MaturityDate
	expiryDate := context.ExpiryDate
	grants := grant.CreateGrants(signer, promotionUUID, *numGrants, altCurrency, *value, maturityDate, expiryDate)
	var grantReg grantRegistration
	grantReg.Grants = grants
	grantReg.Promotions = []promotionInfo{{ID: promotionUUID, Priority: 0, Active: false, MinimumReconcileTimestamp: maturityDate.Unix() * 1000}}
	serializedGrants, err := json.Marshal(grantReg)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(*outputFile, serializedGrants, 0600)
	if err != nil {
		return err
	}
	return nil
}
