package inputs

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"

	errorutils "github.com/brave-intl/bat-go/libs/errors"
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

// DecodeAndValidateReader - perform decode and validate of input in one swipe
func DecodeAndValidateReader(ctx context.Context, v DecodeValidate, input io.Reader) error {
	b, err := ioutil.ReadAll(input)
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}
	return DecodeAndValidate(ctx, v, b)
}

// DecodeAndValidate - perform decode and validate of input in one swipe
func DecodeAndValidate(ctx context.Context, v DecodeValidate, input []byte) error {
	var me = new(errorutils.MultiError)
	if err := v.Decode(ctx, input); err != nil {
		me.Append(fmt.Errorf("failed decoding: %w", err))
	}
	if err := v.Validate(ctx); err != nil {
		me.Append(fmt.Errorf("failed validation: %w", err))
	}
	if me.Count() > 0 {
		return me
	}
	return nil
}
