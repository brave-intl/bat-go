package vaultsigner

import (
	"crypto/rand"
	"encoding/base64"
	"time"

	"github.com/hashicorp/vault/api"
	util "github.com/hashicorp/vault/api/cliconfig"
	"github.com/hashicorp/vault/sdk/helper/jsonutil"
	"github.com/hashicorp/vault/sdk/helper/keysutil"
	"golang.org/x/crypto/ed25519"
)

// WrappedClient holds an api client for interacting with vault
type WrappedClient struct {
	Client *api.Client
}

// FromKeypair create a new vault transit key by importing privKey and pubKey under importName
func (wc *WrappedClient) FromKeypair(privKey ed25519.PrivateKey, pubKey ed25519.PublicKey, importName string) (*Ed25519Signer, error) {
	client := wc.Client
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

	key.CreationTime = time.Now().UTC()
	key.DeprecatedCreationTime = key.CreationTime.Unix()

	keyData := keysutil.KeyData{Policy: &keysutil.Policy{Keys: map[string]keysutil.KeyEntry{"1": key}}}

	keyData.Policy.ArchiveVersion = 1
	keyData.Policy.BackupInfo = &keysutil.BackupInfo{Time: time.Now().UTC(), Version: 1}
	keyData.Policy.LatestVersion = 1
	keyData.Policy.MinDecryptionVersion = 1
	keyData.Policy.Name = importName
	keyData.Policy.Type = keysutil.KeyType_ED25519

	encodedBackup, err := jsonutil.EncodeJSON(keyData)
	if err != nil {
		return nil, err
	}
	backup := base64.StdEncoding.EncodeToString(encodedBackup)

	err = wc.GenerateMounts()
	if err != nil {
		return nil, err
	}

	// Restore the generated key backup
	_, err = client.Logical().Write("transit/restore", map[string]interface{}{
		"backup": backup,
		"name":   importName,
	})
	if err != nil {
		return nil, err
	}

	return &Ed25519Signer{Client: client, KeyName: importName, KeyVersion: 1}, nil
}

// ImportHmacSecret create a new vault transit key by importing privKey under importName
func (wc *WrappedClient) ImportHmacSecret(secret []byte, importName string) (*HmacSigner, error) {
	client := wc.Client
	key := keysutil.KeyEntry{}

	key.HMACKey = secret
	key.Key = secret

	key.CreationTime = time.Now().UTC()
	key.DeprecatedCreationTime = key.CreationTime.Unix()

	keyData := keysutil.KeyData{Policy: &keysutil.Policy{Keys: map[string]keysutil.KeyEntry{"1": key}}}

	keyData.Policy.ArchiveVersion = 1
	keyData.Policy.BackupInfo = &keysutil.BackupInfo{Time: time.Now().UTC(), Version: 1}
	keyData.Policy.LatestVersion = 1
	keyData.Policy.MinDecryptionVersion = 1
	keyData.Policy.Name = importName
	keyData.Policy.Exportable = true
	keyData.Policy.AllowPlaintextBackup = true

	encodedBackup, err := jsonutil.EncodeJSON(keyData)
	if err != nil {
		return nil, err
	}
	backup := base64.StdEncoding.EncodeToString(encodedBackup)

	err = wc.GenerateMounts()
	if err != nil {
		return nil, err
	}

	// Restore the generated key backup
	_, err = client.Logical().Write("transit/restore", map[string]interface{}{
		"backup": backup,
		"name":   importName,
	})
	if err != nil {
		return nil, err
	}

	return &HmacSigner{Client: client, KeyName: importName, KeyVersion: 1}, nil
}

// GenerateMounts generates the appropriate mount points if they do not exist
func (wc *WrappedClient) GenerateMounts() error {
	mounts, err := wc.Client.Sys().ListMounts()
	if err != nil {
		return err
	}
	if _, ok := mounts["wallets/"]; !ok {
		// Mount kv secret backend if not already mounted
		if err = wc.Client.Sys().Mount("wallets", &api.MountInput{
			Type: "kv",
		}); err != nil {
			return err
		}
	}
	if _, ok := mounts["transit/"]; !ok {
		// Mount transit secret backend if not already mounted
		if err = wc.Client.Sys().Mount("transit", &api.MountInput{
			Type: "transit",
		}); err != nil {
			return err
		}
	}
	return nil
}

// GenerateEd25519Signer create Ed25519Signer by generating a keypair with name using vault backend
func (wc *WrappedClient) GenerateEd25519Signer(name string) (*Ed25519Signer, error) {
	err := wc.GenerateMounts()
	if err != nil {
		return nil, err
	}
	// Generate a new keypair
	_, err = wc.Client.Logical().Write("transit/keys/"+name, map[string]interface{}{
		"type": "ed25519",
	})
	if err != nil {
		return nil, err
	}

	return &Ed25519Signer{Client: wc.Client, KeyName: name, KeyVersion: 1}, nil
}

// GetEd25519Signer gets a key pair but doesn't generate new key
func (wc *WrappedClient) GetEd25519Signer(name string) (*Ed25519Signer, error) {
	err := wc.GenerateMounts()
	if err != nil {
		return nil, err
	}

	return &Ed25519Signer{Client: wc.Client, KeyName: name, KeyVersion: 1}, nil
}

// GenerateHmacSecret create hmac key using vault backend
func (wc *WrappedClient) GenerateHmacSecret(name string, algo string) (*HmacSigner, error) {
	err := wc.GenerateMounts()
	if err != nil {
		return nil, err
	}

	// Generate a new hmac set
	_, err = wc.Client.Logical().Write("transit/hmac/"+name+"/"+algo, nil)
	if err != nil {
		return nil, err
	}

	return &HmacSigner{Client: wc.Client, KeyName: name, KeyVersion: 1}, nil
}

// GetHmacSecret gets a key pair but doesn't generate new key
func (wc *WrappedClient) GetHmacSecret(name string) (*HmacSigner, error) {
	err := wc.GenerateMounts()
	if err != nil {
		return nil, err
	}

	return &HmacSigner{Client: wc.Client, KeyName: name, KeyVersion: 1}, nil
}

// Connect connects to the vaultsigner backend server, sets token written by vault
func Connect() (*WrappedClient, error) {
	var client *api.Client
	config := &api.Config{}
	err := config.ReadEnvironment()

	if err == nil {
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
			if token != "" {
				client.SetToken(token)
			}
		}
	}

	return &WrappedClient{Client: client}, err
}
