package vaultsigner

import (
	"crypto"
	"crypto/rand"
	"os"
	"testing"

	uuid "github.com/hashicorp/go-uuid"
	"github.com/hashicorp/vault/api"
	"golang.org/x/crypto/ed25519"
)

func TestSign(t *testing.T) {
	if os.Getenv("VAULT_TOKEN") == "" {
		t.Skip("skipping test; VAULT_TOKEN not set")
	}

	config := &api.Config{}
	err := config.ReadEnvironment()

	var client *api.Client
	if err != nil {
		client, err = api.NewClient(config)
	} else {
		client, err = api.NewClient(nil)
		client.SetAddress("http://127.0.0.1:8200")
	}

	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	name, err := uuid.GenerateUUID()
	if err != nil {
		t.Fatal(err)
	}

	signer, err := FromKeypair(client, privateKey, publicKey, "vaultsigner-test-"+name)
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

	config := &api.Config{}
	err := config.ReadEnvironment()

	var client *api.Client
	if err != nil {
		client, err = api.NewClient(config)
	} else {
		client, err = api.NewClient(nil)
		client.SetAddress("http://127.0.0.1:8200")
	}

	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	name, err := uuid.GenerateUUID()
	if err != nil {
		t.Fatal(err)
	}

	signer, err := FromKeypair(client, privateKey, publicKey, "vaultsigner-test-"+name)
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
