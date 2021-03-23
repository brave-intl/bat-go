package inputs

import (
	"context"
	"errors"
)

var (
	// ErrInvalidCustodian - an error that tells caller this is not valid
	ErrInvalidCustodian = errors.New("failed to validate custodian")
	// ErrCustodianDecodeEmpty - an error that tells caller this is empty and should not be
	ErrCustodianDecodeEmpty = errors.New("failed to decode custodian: custodian cannot be empty")
)

// Custodian - wallet custodian
type Custodian struct {
	raw string
}

// String - return the String representation of the ID
func (c *Custodian) String() string {
	return c.raw
}

// Validate - take raw []byte input and populate id with the ID
func (c *Custodian) Validate(ctx context.Context) error {
	// this should be overloaded to validate ids are real...
	switch c.raw {
	case "uphold", "brave", "bitflyer":
		return nil
	default:
		return ErrInvalidCustodian
	}
}

// Decode - take raw []byte input and populate id with the ID
func (c *Custodian) Decode(ctx context.Context, input []byte) error {
	var err error

	if len(input) == 0 {
		return ErrIDDecodeEmpty
	}
	c.raw = string(input)

	return err
}
