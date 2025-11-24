//go:build integration

package vaultsigner

import (
	"crypto"
	"crypto/rand"
	"testing"

	uuid "github.com/satori/go.uuid"
	"golang.org/x/crypto/ed25519"
)

func TestSign(t *testing.T) {
	wrappedClient, err := Connect()
	if err != nil {
		t.Fatal(err)
	}

	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	name := uuid.NewV4()

	signer, err := wrappedClient.FromKeypair(privateKey, publicKey, "vaultsigner-test-"+name.String())
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
	wrappedClient, err := Connect()
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

	signer, err := wrappedClient.FromKeypair(privateKey, publicKey, "vaultsigner-test-"+name.String())
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
