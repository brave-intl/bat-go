package vaultsigner

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/hashicorp/vault/api"
)

// HmacSigner signer / verifier that uses the vault transit backend
type HmacSigner struct {
	Client     *api.Client
	KeyName    string
	KeyVersion uint
}

// HMACSha384 the included message using the vault held keypair
func (vs *HmacSigner) HMACSha384(message []byte) ([]byte, error) {
	response, err := vs.Client.Logical().Write("transit/hmac/"+vs.KeyName+"/sha2-384", map[string]interface{}{
		"input": base64.StdEncoding.EncodeToString(message),
	})
	if err != nil {
		return []byte{}, err
	}

	hmac := response.Data["hmac"].(string)
	fmt.Println(hmac)

	return base64.StdEncoding.DecodeString(strings.Split(hmac, ":")[2])
}

// Verify the included signature over message using the vault held keypair
func (vs *HmacSigner) Verify(message, signature []byte) (bool, error) {
	response, err := vs.Client.Logical().Write("transit/verify/"+vs.KeyName, map[string]interface{}{
		"input":     base64.StdEncoding.EncodeToString(message),
		"signature": fmt.Sprintf("vault:v%d:%s", vs.KeyVersion, base64.StdEncoding.EncodeToString(signature)),
	})
	if err != nil {
		return false, err
	}

	return response.Data["valid"].(bool), nil
}
