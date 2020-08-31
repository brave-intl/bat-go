package vaultsigner

import (
	"crypto"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"github.com/hashicorp/vault/api"
)

// HmacSigner signer / verifier that uses the vault transit backend
type HmacSigner struct {
	Client     *api.Client
	KeyName    string
	KeyVersion uint
}

// Sign the included message using the vault held keypair. rand and opts are not used
func (vs *HmacSigner) Sign(rand io.Reader, message []byte, opts crypto.SignerOpts) ([]byte, error) {
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
func (vs *HmacSigner) Verify(message, signature []byte, opts crypto.SignerOpts) (bool, error) {
	response, err := vs.Client.Logical().Write("transit/verify/"+vs.KeyName, map[string]interface{}{
		"input":     base64.StdEncoding.EncodeToString(message),
		"signature": fmt.Sprintf("vault:v%d:%s", vs.KeyVersion, base64.StdEncoding.EncodeToString(signature)),
	})
	if err != nil {
		return false, err
	}

	return response.Data["valid"].(bool), nil
}
