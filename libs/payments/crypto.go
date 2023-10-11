package payments

import (
	"context"
	"crypto"
	"io"
)

// Signator is an interface for cryptographic signature creation
// NOTE that this is a subset of the crypto.Signer interface
type Signator interface {
	Sign(rand io.Reader, digest []byte, opts crypto.SignerOpts) (signature []byte, err error)
}

// Verifier is an interface for verifying signatures
type Verifier interface {
	Verify(message, sig []byte, opts crypto.SignerOpts) (bool, error)
}

// Keystore provides a way to lookup a public key based on the keyID a request was signed with
type Keystore interface {
	// LookupVerifier based on the keyID
	LookupVerifier(ctx context.Context, keyID string) (context.Context, *Verifier, error)
}
