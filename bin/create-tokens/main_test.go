// +build integration

package main

import (
	"os"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/grant"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/vaultsigner"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"golang.org/x/crypto/ed25519"
)

func init() {
	os.Setenv("ENV", "production")
}

func TestNewJoseVaultSigner(t *testing.T) {
	client, err := vaultsigner.Connect()
	if err != nil {
		t.Fatal(err)
	}

	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	name := uuid.NewV4()

	vSigner, err := vaultsigner.FromKeypair(client, privateKey, publicKey, "create-tokens-test-"+name.String())
	if err != nil {
		t.Fatal(err)
	}

	signer, err := newJoseVaultSigner(vSigner)
	if err != nil {
		t.Error(err)
	}

	maturityDate := time.Now()
	// + 1 week
	expiryDate := maturityDate.AddDate(0, 0, 1)

	altCurrency := altcurrency.BAT

	grantTemplate := grant.Grant{
		AltCurrency:       &altCurrency,
		Probi:             altcurrency.BAT.ToProbi(decimal.NewFromFloat(30)),
		PromotionID:       uuid.NewV4(),
		MaturityTimestamp: maturityDate.Unix(),
		ExpiryTimestamp:   expiryDate.Unix(),
	}

	grants, err := grant.CreateGrants(signer, grantTemplate, 1)
	if err != nil {
		t.Error(err)
	}
	_, err = grant.DecodeGrants(vSigner.Public().(ed25519.PublicKey), grants)
	if err != nil {
		t.Error(err)
	}
}
