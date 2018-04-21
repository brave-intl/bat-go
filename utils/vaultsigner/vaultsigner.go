package vaultsigner

import (
	"crypto"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"github.com/hashicorp/vault/api"
)

type VaultSigner struct {
	Client     *api.Client
	KeyName    string
	KeyVersion uint
}

func (vs *VaultSigner) Sign(rand io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	response, err := vs.Client.Logical().Write("transit/sign/"+vs.KeyName, map[string]interface{}{
		"input": base64.StdEncoding.EncodeToString(digest),
	})
	if err != nil {
		return []byte{}, err
	}

	sig := response.Data["signature"].(string)

	return base64.StdEncoding.DecodeString(strings.Split(sig, ":")[2])
}

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
