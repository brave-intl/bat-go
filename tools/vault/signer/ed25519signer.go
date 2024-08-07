package vaultsigner

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/hashicorp/vault/api"
)

// Ed25519Signer signer / verifier that uses the vault transit backend
type Ed25519Signer struct {
	Client     *api.Client
	KeyName    string
	KeyVersion uint
}

// Sign the included message using the vault held keypair. rand and opts are not used
func (vs *Ed25519Signer) SignMessage(message []byte) ([]byte, error) {
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
func (vs *Ed25519Signer) VerifySignature(message, signature []byte) error {
	response, err := vs.Client.Logical().Write("transit/verify/"+vs.KeyName, map[string]interface{}{
		"input":     base64.StdEncoding.EncodeToString(message),
		"signature": fmt.Sprintf("vault:v%d:%s", vs.KeyVersion, base64.StdEncoding.EncodeToString(signature)),
	})
	if err != nil {
		return err
	}

	valid := response.Data["valid"].(bool)
	if !valid {
		return httpsignature.ErrBadSignature
	}
	return nil
}

// String returns the public key as a hex encoded string
func (vs *Ed25519Signer) String() string {
	return hex.EncodeToString(vs.Public())
}

// Public returns the public key
func (vs *Ed25519Signer) Public() httpsignature.Ed25519PubKey {
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

	return httpsignature.Ed25519PubKey(publicKey)
}
