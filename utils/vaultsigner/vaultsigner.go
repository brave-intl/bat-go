package vaultsigner

import (
	"crypto"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/vault/api"
	util "github.com/hashicorp/vault/command/config"
	"github.com/hashicorp/vault/helper/jsonutil"
	"github.com/hashicorp/vault/helper/keysutil"
	"golang.org/x/crypto/ed25519"
)

// VaultSigner an ed25519 signer / verifier that uses the vault transit backend
type VaultSigner struct {
	Client     *api.Client
	KeyName    string
	KeyVersion uint
}

// Sign the included message using the vault held keypair. rand and opts are not used
func (vs *VaultSigner) Sign(rand io.Reader, message []byte, opts crypto.SignerOpts) ([]byte, error) {
	response, err := vs.Client.Logical().Write("transit/sign/"+vs.KeyName, map[string]interface{}{
		"input": base64.StdEncoding.EncodeToString(message),
	})
	if err != nil {
		return []byte{}, err
	}

	sig := response.Data["signature"].(string)

	return base64.StdEncoding.DecodeString(strings.Split(sig, ":")[2])
}

// Verify the included signature over message using the vault held keypair. opts are not used
func (vs *VaultSigner) Verify(message, signature []byte, opts crypto.SignerOpts) (bool, error) {
	response, err := vs.Client.Logical().Write("transit/verify/"+vs.KeyName, map[string]interface{}{
		"input":     base64.StdEncoding.EncodeToString(message),
		"signature": fmt.Sprintf("vault:v%d:%s", vs.KeyVersion, base64.StdEncoding.EncodeToString(signature)),
	})
	if err != nil {
		return false, err
	}

	return response.Data["valid"].(bool), nil
}

// String returns the public key as a hex encoded string
func (vs *VaultSigner) String() string {
	return hex.EncodeToString(vs.Public().(ed25519.PublicKey))
}

// Public returns the public key
func (vs *VaultSigner) Public() crypto.PublicKey {
	response, err := vs.Client.Logical().Read("transit/keys/" + vs.KeyName)
	if err != nil {
		panic(err)
	}

	keys := response.Data["keys"].(map[string]interface{})
	key := keys[strconv.Itoa(int(vs.KeyVersion))].(map[string]interface{})
	b64PublicKey := key["public_key"].(string)
	publicKey, err := base64.StdEncoding.DecodeString(b64PublicKey)
	if err != nil {
		panic(err)
	}

	return ed25519.PublicKey(publicKey)
}

// FromKeypair create a new vault transit key by importing privKey and pubKey under importName
func FromKeypair(client *api.Client, privKey ed25519.PrivateKey, pubKey ed25519.PublicKey, importName string) (*VaultSigner, error) {
	key := keysutil.KeyEntry{}

	key.Key = privKey

	pk := base64.StdEncoding.EncodeToString(pubKey)
	key.FormattedPublicKey = pk

	{
		tmp := make([]byte, 32)
		_, err := rand.Read(tmp)
		if err != nil {
			return nil, err
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
	keyData.Policy.Name = importName
	keyData.Policy.Type = keysutil.KeyType_ED25519

	encodedBackup, err := jsonutil.EncodeJSON(keyData)
	if err != nil {
		return nil, err
	}
	backup := base64.StdEncoding.EncodeToString(encodedBackup)

	mounts, err := client.Sys().ListMounts()
	if err != nil {
		return nil, err
	}
	if _, ok := mounts["transit/"]; !ok {
		// Mount transit secret backend if not already mounted
		if err = client.Sys().Mount("transit", &api.MountInput{
			Type: "transit",
		}); err != nil {
			return nil, err
		}
	}

	// Restore the generated key backup
	_, err = client.Logical().Write("transit/restore", map[string]interface{}{
		"backup": backup,
	})
	if err != nil {
		return nil, err
	}

	return &VaultSigner{Client: client, KeyName: importName, KeyVersion: 1}, nil
}

// New VaultSigner by generating a keypair with name using vault backend
func New(client *api.Client, name string) (*VaultSigner, error) {
	mounts, err := client.Sys().ListMounts()
	if err != nil {
		return nil, err
	}
	if _, ok := mounts["transit/"]; !ok {
		// Mount transit secret backend if not already mounted
		if err = client.Sys().Mount("transit", &api.MountInput{
			Type: "transit",
		}); err != nil {
			return nil, err
		}
	}

	// Generate a new keypair
	_, err = client.Logical().Write("transit/keys/"+name, map[string]interface{}{
		"type": "ed25519",
	})
	if err != nil {
		return nil, err
	}

	return &VaultSigner{Client: client, KeyName: name, KeyVersion: 1}, nil
}

// Connect connects to the vaultsigner backend server, sets token written by vault
func Connect() (client *api.Client, err error) {
	config := &api.Config{}
	err = config.ReadEnvironment()

	if err != nil {
		client, err = api.NewClient(config)
	} else {
		client, err = api.NewClient(nil)
		if err != nil {
			return nil, err
		}
		err = client.SetAddress("http://127.0.0.1:8200")
	}
	if err != nil {
		return nil, err
	}

	helper, err := util.DefaultTokenHelper()
	if err == nil {
		var token string
		token, err = helper.Get()
		if err == nil {
			client.SetToken(token)
		}
	}

	return client, err
}
