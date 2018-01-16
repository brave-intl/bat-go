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
	grantSignatorPublicKeyHex = os.Getenv("GRANT_SIGNATOR_PUBLIC_KEY")
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
	if len(grantSignatorPublicKeyHex) == 0 {
		log.Fatalln("Must pass grant signing key via env var GRANT_SIGNATOR_PRIVATE_KEY")
	}
	err := grant.InitGrantService(nil)
	if err != nil {
		log.Fatalln(err)
	}

	contents, err := ioutil.ReadFile(*inputFile)
	if err != nil {
		log.Fatalln(err)
	}

	var grantReg grantRegistration
	err = json.Unmarshal(contents, &grantReg)
	if err != nil {
		log.Fatalln(err)
	}

	for i := 0; i < len(grantReg.Grants); i++ {
		var g *grant.Grant
		g, err = grant.FromCompactJWS(grantReg.Grants[i])
		if err != nil {
			log.Fatalln(err)
		}
		if g.PromotionID != grantReg.Promotions[0].ID {
			log.Fatalln("promotion mismatch")
		}
		_, err = grantIds.Add(g.GrantID.String())
		if err != nil {
			log.Fatalln(err)
		}
	}
	numIds, err := grantIds.Cardinality()
	if err != nil {
		log.Fatalln(err)
	}
	log.Printf("Success! All grants passed verification, %d unique grants seen\n", numIds)
}
