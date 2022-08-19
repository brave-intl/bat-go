package inputs

import (
	"bytes"
	"context"
	"encoding/json"
)

// Decodable - and interface that allows for validation of inputs and params
type Decodable interface {
	Decode(context.Context, []byte) error
}

// Decode - decode a decodable thing
func Decode(ctx context.Context, d Decodable, input []byte) error {
	return d.Decode(ctx, input)
}

// DecodeJSON - decode json helper
func DecodeJSON(ctx context.Context, input []byte, v interface{}) error {
	dec := json.NewDecoder(bytes.NewBuffer(input))
	return dec.Decode(v)
}
