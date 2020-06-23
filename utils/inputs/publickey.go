package inputs

import (
	"context"
	"errors"
)

var (
	// ErrPublicKeyDecodeEmpty - an error that tells caller the public key is empty and should not be
	ErrPublicKeyDecodeEmpty = errors.New("failed to decode public key: public key cannot be empty")
)

// PublicKey - a generic ID type that can be used for common id based things
type PublicKey string

// String - return the String representation of the ID
func (pk *PublicKey) String() string {
	return string(*pk)
}

// Validate - take raw []byte input and populate id with the ID
func (pk *PublicKey) Validate(ctx context.Context) error {
	// this should be overloaded to validate ids are real...
	return nil
}

// Decode - take raw []byte input and populate id with the ID
func (pk *PublicKey) Decode(ctx context.Context, input []byte) error {
	if len(input) == 0 {
		return ErrPublicKeyDecodeEmpty
	}
	*pk = PublicKey(string(input))
	return nil
}
