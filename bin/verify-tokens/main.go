package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"os"

	"github.com/brave-intl/bat-go/grant"
	"github.com/brave-intl/bat-go/utils/set"
	"github.com/satori/go.uuid"
)

var (
	GrantSignatorPublicKeyHex = os.Getenv("GRANT_SIGNATOR_PUBLIC_KEY")
	inputFile                 = flag.String("in", "./grantTokens.json", "input file path")
	grantIds                  = set.NewSliceSet()
)

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
	if len(GrantSignatorPublicKeyHex) == 0 {
		log.Fatalln("Must pass grant signing key via env var GRANT_SIGNATOR_PRIVATE_KEY")
	}
	grant.InitGrantService()

	contents, err := ioutil.ReadFile(*inputFile)
	if err != nil {
		log.Fatalln(err)
	}

	var grantReg grantRegistration
	err = json.Unmarshal(contents, &grantReg)
	if err != nil {
	}

	for i := 0; i < len(grantReg.Grants); i++ {
		grant, err := grant.FromCompactJWS(grantReg.Grants[i])
		if err != nil {
			log.Fatalln(err)
		}
		if grant.PromotionId != grantReg.Promotions[0].ID {
			log.Fatalln("promotion mismatch")
		}
		grantIds.Add(grant.GrantId.String())
	}
	numIds, err := grantIds.Cardinality()
	if err != nil {
		log.Fatalln(err)
	}
	log.Printf("Success! All grants passed verification, %d unique grants seen\n", numIds)
}
