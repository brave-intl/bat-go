package payments

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"os"
)

// GetOperatorPrivateKey - get the private key from the file specified
func GetOperatorPrivateKey(filename string) (ed25519.PrivateKey, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open key file: %w", err)
	}
	defer f.Close()

	privateKeyPEM, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("failed to read key file: %w", err)
	}

	p, _ := pem.Decode(privateKeyPEM)
	key, err := x509.ParsePKCS8PrivateKey(p.Bytes)
	if err != nil {
		return nil, err
	}

	edKey, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("key is not ed25519 key")
	}

	return edKey, nil
}
