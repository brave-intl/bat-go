package payments

import (
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
