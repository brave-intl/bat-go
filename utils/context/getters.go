package context

import (
	"context"

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
	v := ctx.Value(LoggerCTXKey)
	if v == nil {
		// value not on context
		return nil, ErrNotInContext
	}
	if s, ok := v.(*zerolog.Logger); ok {
		return s, nil
	}
	// value not a string
	return nil, ErrValueWrongType
}
