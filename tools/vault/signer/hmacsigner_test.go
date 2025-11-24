//go:build integration

package vaultsigner

import (
	"encoding/hex"
	"testing"

	"github.com/brave-intl/bat-go/libs/cryptography"
	uuid "github.com/satori/go.uuid"
)

func TestHmacSign(t *testing.T) {
	wrappedClient, err := Connect()
	if err != nil {
		t.Fatal(err)
	}

	name := "vaultsigner-hmactest-" + uuid.NewV4().String()

	secret := []byte("mysecret")
	data := []byte("hello world")

	// Create a new HMAC by defining the hash type and the key (as byte array)
	h := cryptography.NewHMACHasher(secret)
	inMemorySha, err := h.HMACSha384(data)
	if err != nil {
		t.Fatal(err)
	}
	hexInMemorySha := hex.EncodeToString(inMemorySha)

	_, err = wrappedClient.ImportHmacSecret(secret, name)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := wrappedClient.GetHmacSecret(name)
	if err != nil {
		t.Fatal(err)
	}

	vaultSha, err := signer.HMACSha384(data)
	if err != nil {
		t.Fatal(err)
	}
	hexVaultSha := hex.EncodeToString(vaultSha)

	if hexVaultSha != hexInMemorySha {
		t.Fatalf("shas did not match:\n%s\n%s", hexVaultSha, hexInMemorySha)
	}
}
