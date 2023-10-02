package payments

import (
	"crypto"
)

// Verifier is an interface for verifying signatures
type Verifier interface {
	Verify(message, sig []byte, opts crypto.SignerOpts) (bool, error)
}
