package inputs

import (
	"context"
	"fmt"
)

// DecodeValidate - decode and validate for inputs
type DecodeValidate interface {
	Validatable
	Decodable
}

// DecodeAndValidateString - perform decode and validate of input in one swipe of a string input
func DecodeAndValidateString(ctx context.Context, v DecodeValidate, input string) error {
	return DecodeAndValidate(ctx, v, []byte(input))
}

// DecodeAndValidate - perform decode and validate of input in one swipe
func DecodeAndValidate(ctx context.Context, v DecodeValidate, input []byte) error {
	if err := v.Decode(ctx, input); err != nil {
		return fmt.Errorf("failed decoding: %w", err)
	}
	if err := v.Validate(ctx); err != nil {
		return fmt.Errorf("failed validation: %w", err)
	}
	return nil
}
