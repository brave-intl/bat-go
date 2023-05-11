package vaultsigner

import (
	"crypto"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/hashicorp/vault/api"
	"golang.org/x/crypto/ed25519"
)

// Ed25519Signer signer / verifier that uses the vault transit backend
type Ed25519Signer struct {
	Client     *api.Client
	KeyName    string
	KeyVersion uint
}

// Sign the included message using the vault held keypair. rand and opts are not used
func (vs *Ed25519Signer) Sign(rand io.Reader, message []byte, opts crypto.SignerOpts) ([]byte, error) {
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
func (vs *Ed25519Signer) Verify(message, signature []byte, opts crypto.SignerOpts) (bool, error) {
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
func (vs *Ed25519Signer) String() string {
	return hex.EncodeToString(vs.Public().(ed25519.PublicKey))
}

// Public returns the public key
func (vs *Ed25519Signer) Public() crypto.PublicKey {
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
