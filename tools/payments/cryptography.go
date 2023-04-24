package payments

import (
	"crypto/ed25519"
	"encoding/asn1"
	"encoding/pem"
	"fmt"
	"io"
	"os"
)

type ed25519PrivKey struct {
	Version          int
	ObjectIdentifier struct {
		ObjectIdentifier asn1.ObjectIdentifier
	}
	PrivateKey []byte
}

// GetOperatorPrivateKey - get the private key from the file specified
func GetOperatorPrivateKey(filename string) (ed25519.PrivateKey, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open key file: %w", err)
	}

	privateKeyPEM, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("failed to read key file: %w", err)
	}

	var block *pem.Block
	block, _ = pem.Decode(privateKeyPEM)

	var asn1PrivKey ed25519PrivKey
	if _, err := asn1.Unmarshal(block.Bytes, &asn1PrivKey); err != nil {
		return nil, fmt.Errorf("failed to unmarshal pem key file: %w", err)
	}

	return ed25519.NewKeyFromSeed(asn1PrivKey.PrivateKey[2:]), nil
}
