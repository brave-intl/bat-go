// +build integration

package vaultsigner

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"testing"

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
	fmt.Printf("Secret: %s Data: %s\n", secret, data)

	// Create a new HMAC by defining the hash type and the key (as byte array)
	h := hmac.New(sha256.New, secret)

	// Write Data to it
	h.Write([]byte(data))

	// Get result and encode as hexadecimal string
	hexInMemorySha := hex.EncodeToString(h.Sum(nil))

	_, err = wrappedClient.ImportHmacSecret([]byte(base64.StdEncoding.EncodeToString(secret)), name)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := wrappedClient.GenerateHmacSecret(name, "sha2-384")
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

// func TestVerify(t *testing.T) {
// 	wrappedClient, err := Connect()
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	publicKey, privateKey, err := ed25519.GenerateKey(nil)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	name := uuid.NewV4()
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	signer, err := wrappedClient.FromKeypair(privateKey, publicKey, "vaultsigner-test-"+name.String())
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	message := []byte("hello world")

// 	signature, err := privateKey.Sign(rand.Reader, message, crypto.Hash(0))
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	valid, err := signer.Verify(message, signature, crypto.Hash(0))
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	if !valid {
// 		t.Fatal("Signature should be valid")
// 	}
// }
