// +build integration

package main

import (
	"os"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/grant"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/vaultsigner"
	"github.com/hashicorp/vault/api"
	uuid "github.com/satori/go.uuid"
	"golang.org/x/crypto/ed25519"
)

func init() {
	os.Setenv("ENV", "production")
}

func TestNewJoseVaultSigner(t *testing.T) {
	if os.Getenv("VAULT_TOKEN") == "" {
		t.Skip("skipping test; VAULT_TOKEN not set")
	}

	config := &api.Config{Address: "http://127.0.0.1:8200"}
	client, err := api.NewClient(config)
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

	grants, err := grant.CreateGrants(signer, uuid.NewV4(), 1, altcurrency.BAT, 30, maturityDate, expiryDate)
	if err != nil {
		t.Error(err)
	}
	_, err = grant.DecodeGrants(vSigner.Public().(ed25519.PublicKey), grants)
	if err != nil {
		t.Error(err)
	}
}
