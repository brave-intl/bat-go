package inputs

import "context"

// Validatable - and interface that allows for validation of inputs and params
type Validatable interface {
	Validate(context.Context) error
}

// Validate - a validatable thing
func Validate(ctx context.Context, v Validatable) error {
	return v.Validate(ctx)
}
