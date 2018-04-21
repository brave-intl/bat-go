package vaultsigner

import (
	"crypto"
	"crypto/rand"
	"encoding/base64"
	"os"
	"testing"

	"github.com/hashicorp/vault/api"
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

	signer := VaultSigner{Client: client, KeyName: "my-import-test-3"}

	signature, err := signer.Sign(rand.Reader, []byte("hello world"), crypto.Hash(0))
	if err != nil {
		t.Fatal(err)
	}

	if base64.StdEncoding.EncodeToString(signature) != "YbNDWC6ZROZqnvHDoU0vUGClws5Gf6S/fuXizTGqZkOvbTOv1xBh84pZ5xuZTs7rz/Qdr3ngtfzpyPmUV+C3Ag==" {
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

	signer := VaultSigner{Client: client, KeyName: "my-import-test-3", KeyVersion: 1}

	signature, err := base64.StdEncoding.DecodeString("YbNDWC6ZROZqnvHDoU0vUGClws5Gf6S/fuXizTGqZkOvbTOv1xBh84pZ5xuZTs7rz/Qdr3ngtfzpyPmUV+C3Ag==")
	if err != nil {
		t.Fatal(err)
	}

	valid, err := signer.Verify([]byte("hello world"), []byte(signature), crypto.Hash(0))
	if err != nil {
		t.Fatal(err)
	}
	if !valid {
		t.Fatal("Signature should be valid")
	}
}
