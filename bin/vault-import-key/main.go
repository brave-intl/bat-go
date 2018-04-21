package main

import (
	"encoding/base64"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/hashicorp/vault/api"
	"github.com/hashicorp/vault/helper/jsonutil"
	"github.com/hashicorp/vault/helper/keysutil"
	"golang.org/x/crypto/ed25519"

	uuid "github.com/hashicorp/go-uuid"
)

var privateKeyHex = os.Getenv("ED25519_PRIVATE_KEY")
var publicKeyHex = os.Getenv("ED25519_PUBLIC_KEY")

// CreateVaultCompatibleBackup from ED25519 private / public keypair
func CreateVaultCompatibleBackup(privKey ed25519.PrivateKey, pubKey ed25519.PublicKey, name string) string {
	key := keysutil.KeyEntry{}

	key.Key = privKey

	pk := base64.StdEncoding.EncodeToString(pubKey)
	key.FormattedPublicKey = pk

	{
		tmp, err := uuid.GenerateRandomBytes(32)
		if err != nil {
			log.Fatalln(err)
		}
		key.HMACKey = tmp
	}

	key.CreationTime = time.Now()
	key.DeprecatedCreationTime = key.CreationTime.Unix()

	keyData := keysutil.KeyData{Policy: &keysutil.Policy{Keys: map[string]keysutil.KeyEntry{"1": key}}}

	keyData.Policy.ArchiveVersion = 1
	keyData.Policy.BackupInfo = &keysutil.BackupInfo{Time: time.Now(), Version: 1}
	keyData.Policy.LatestVersion = 1
	keyData.Policy.MinDecryptionVersion = 1
	keyData.Policy.Name = name
	keyData.Policy.Type = keysutil.KeyType_ED25519

	encodedBackup, err := jsonutil.EncodeJSON(keyData)
	if err != nil {
		log.Fatalln("error:", err)
	}
	return base64.StdEncoding.EncodeToString(encodedBackup)
}

func main() {
	log.SetFlags(0)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "A helper for importing existing ed25519 keys into vault.\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n\n")
		fmt.Fprintf(os.Stderr, "        %s VAULT_KEY_NAME\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  Hex key material is read from the environment, ED25519_PUBLIC_KEY and ED25519_PRIVATE_KEY.\n\n")
	}
	flag.Parse()

	if len(privateKeyHex) == 0 || len(publicKeyHex) == 0 {
		fmt.Fprintf(os.Stderr, "ERROR: Environment variables ED25519_PRIVATE_KEY and ED25519_PUBLIC_KEY must be passed\n\n")
		flag.Usage()
		os.Exit(1)
	}

	args := flag.Args()
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "ERROR: Must pass a single argument to name imported keypair\n\n")
		flag.Usage()
		os.Exit(1)
	}

	privKey, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		log.Fatalln("ERROR: Key material must be passed as hex")
	}

	pubKey, err := hex.DecodeString(publicKeyHex)
	if err != nil {
		log.Fatalln("ERROR: Key material must be passed as hex")
	}

	backup := CreateVaultCompatibleBackup(privKey, pubKey, args[0])

	config := &api.Config{}
	err = config.ReadEnvironment()

	var client *api.Client
	if err != nil {
		client, err = api.NewClient(config)
	} else {
		client, err = api.NewClient(nil)
		client.SetAddress("http://127.0.0.1:8200")
	}

	mounts, err := client.Sys().ListMounts()
	if err != nil {
		log.Fatalln(err)
	}
	if _, ok := mounts["transit/"]; !ok {
		// Mount transit secret backend if not already mounted
		if err := client.Sys().Mount("transit", &api.MountInput{
			Type: "transit",
		}); err != nil {
			log.Fatalln(err)
		}
	}

	// Restore the generated key backup
	_, err = client.Logical().Write("transit/restore", map[string]interface{}{
		"backup": backup,
	})
	if err != nil {
		log.Fatalln(err)
	}
}
