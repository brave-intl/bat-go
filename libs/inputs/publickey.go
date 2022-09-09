package inputs

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
)

var (
	// ErrPublicKeyDecodeEmpty - an error that tells caller the public key is empty and should not be
	ErrPublicKeyDecodeEmpty = errors.New("failed to decode public key: public key cannot be empty")
)

// PublicKey - a generic ID type that can be used for common id based things
type PublicKey string

// String - return the String representation of the public key
func (pk *PublicKey) String() string {
	return string(*pk)
}

// Validate - take raw []byte input and populate public key with the value
func (pk *PublicKey) Validate(ctx context.Context) error {
	_, err := hex.DecodeString(string(*pk))
	if err != nil {
		return fmt.Errorf("invalid public key, not hex encoded: %w", err)
	}
	return nil
}

// Decode - take raw []byte input and populate id with the public key
func (pk *PublicKey) Decode(ctx context.Context, input []byte) error {
	if len(input) == 0 {
		return ErrPublicKeyDecodeEmpty
	}
	*pk = PublicKey(string(input))
	return nil
}
