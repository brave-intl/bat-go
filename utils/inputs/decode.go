package inputs

import "context"

// Decodable - and interface that allows for validation of inputs and params
type Decodable interface {
	Decode(context.Context, []byte) error
}

// Decode - decode a decodable thing
func Decode(ctx context.Context, d Decodable, input []byte) error {
	return d.Decode(ctx, input)
}
