package internal

import (
	"context"
	"crypto/ed25519"
	"encoding/asn1"
	"encoding/pem"
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
func GetOperatorPrivateKey(ctx context.Context, filename string) (ed25519.PrivateKey, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, LogAndError(ctx, err, "GetOperatorPrivateKey", "failed to open key file")
	}

	privateKeyPEM, err := io.ReadAll(f)
	if err != nil {
		return nil, LogAndError(ctx, err, "GetOperatorPrivateKey", "failed to read key file")
	}

	var block *pem.Block
	block, _ = pem.Decode(privateKeyPEM)

	var asn1PrivKey ed25519PrivKey
	if _, err := asn1.Unmarshal(block.Bytes, &asn1PrivKey); err != nil {
		return nil, LogAndError(ctx, err, "GetOperatorPrivateKey", "failed to unmarshal asn1 from pem")
	}

	return ed25519.NewKeyFromSeed(asn1PrivKey.PrivateKey[2:]), nil
}
