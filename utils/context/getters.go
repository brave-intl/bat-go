package context

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"
)

//GetStringFromContext - given a CTXKey return the string value from the context if it exists
func GetStringFromContext(ctx context.Context, key CTXKey) (string, error) {
	v := ctx.Value(key)
	if v == nil {
		// value not on context
		return "", ErrNotInContext
	}
	if s, ok := v.(string); ok {
		return s, nil
	}
	// value not a string
	return "", ErrValueWrongType
}

//GetLogger - return the logger value from the context if it exists
func GetLogger(ctx context.Context) (*zerolog.Logger, error) {
	// get the logger from the context, if the logger is disabled
	// return an error to caller
	var l = zerolog.Ctx(ctx)
	if ll := *l; ll.GetLevel() == zerolog.Disabled {
		// this is a disabled logger, send appropriate error
		return nil, fmt.Errorf("logger not found in context: %w", ErrNotInContext)
	}
	return l, nil
}
