// +build integration

package vaultsigner

import (
	"crypto"
	"crypto/rand"
	"os"
	"testing"

	"github.com/hashicorp/vault/api"
	uuid "github.com/satori/go.uuid"
	"golang.org/x/crypto/ed25519"
)

func TestSign(t *testing.T) {
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

	signer, err := FromKeypair(client, privateKey, publicKey, "vaultsigner-test-"+name.String())
	if err != nil {
		t.Fatal(err)
	}

	message := []byte("hello world")

	signature, err := signer.Sign(rand.Reader, message, crypto.Hash(0))
	if err != nil {
		t.Fatal(err)
	}

	if !ed25519.Verify(publicKey, message, signature) {
		t.Fatal("Signature did not match")
	}
}

func TestVerify(t *testing.T) {
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
	if err != nil {
		t.Fatal(err)
	}

	signer, err := FromKeypair(client, privateKey, publicKey, "vaultsigner-test-"+name.String())
	if err != nil {
		t.Fatal(err)
	}

	message := []byte("hello world")

	signature, err := privateKey.Sign(rand.Reader, message, crypto.Hash(0))
	if err != nil {
		t.Fatal(err)
	}

	valid, err := signer.Verify(message, signature, crypto.Hash(0))
	if err != nil {
		t.Fatal(err)
	}
	if !valid {
		t.Fatal("Signature should be valid")
	}
}
