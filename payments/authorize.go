package payments

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

var (
	// authorizedKeys - this is a mapping of environment to a list of authorized public keys
	// allowed to perform payments related authorizations.
	authorizedKeys = map[string][]string{
		"local": {
			"33a7b54be5cf3487ef92c41580b2e315fce8ed97866ae2ce66807b76b6951cd1",
		},
	}
)

func isSignatureValid(docID, pubKeyHex, sigB64 string) (bool, error) {
	// decode signature
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return false, fmt.Errorf("failed to b64 decode signature: %w", err)
	}
	// decode pubKey
	pubKey, err := hex.DecodeString(pubKeyHex)
	if err != nil {
		return false, fmt.Errorf("failed to hex decode public key: %w", err)
	}

	return ed25519.Verify(ed25519.PublicKey(pubKey), []byte(docID), sig), nil
}

func isKeyValid(key string, keys []string) bool {
	for _, v := range keys {
		if v == key {
			return true
		}
	}
	return false
}
