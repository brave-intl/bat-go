package context

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"
)

//GetByteSliceFromContext - given a CTXKey return the string value from the context if it exists
func GetByteSliceFromContext(ctx context.Context, key CTXKey) ([]byte, error) {
	v := ctx.Value(key)
	if v == nil {
		// value not on context
		return nil, ErrNotInContext
	}
	if s, ok := v.([]byte); ok {
		return s, nil
	}
	// value not a string
	return nil, ErrValueWrongType
}

//GetBoolFromContext - given a CTXKey return the bool value from the context if it exists
func GetBoolFromContext(ctx context.Context, key CTXKey) (bool, error) {
	v := ctx.Value(key)
	if v == nil {
		// value not on context
		return false, ErrNotInContext
	}
	if s, ok := v.(bool); ok {
		return s, nil
	}
	// value not a string
	return false, ErrValueWrongType
}

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

//GetDurationFromContext - given a CTXKey return the duration value from the context if it exists
func GetDurationFromContext(ctx context.Context, key CTXKey) (time.Duration, error) {
	v := ctx.Value(key)
	if v == nil {
		// value not on context
		return time.Duration(0), ErrNotInContext
	}
	if s, ok := v.(time.Duration); ok {
		return s, nil
	}
	// value not a duration
	return time.Duration(0), ErrValueWrongType
}

//GetLogLevelFromContext - given a CTXKey return the duration value from the context if it exists
func GetLogLevelFromContext(ctx context.Context, key CTXKey) (zerolog.Level, error) {
	v := ctx.Value(key)
	if v == nil {
		// value not on context
		return zerolog.InfoLevel, ErrNotInContext
	}
	if l, ok := v.(zerolog.Level); ok {
		return l, nil
	}
	// value not a log level
	return zerolog.InfoLevel, ErrValueWrongType
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
