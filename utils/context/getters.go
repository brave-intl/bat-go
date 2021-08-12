package context

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
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

	// lets make a sublogger with a trace id if it exists on the context
	traceID, err := GetStringFromContext(ctx, TraceIDCTXKey)
	if err != nil {
		if err != ErrNotInContext && err != ErrValueWrongType {
			l.Error().Err(err).Msg("issue getting trace id from context")
		}
	}
	sublogger := l.With().Str("trace_id", traceID).Logger()

	return &sublogger, nil
}

// GetOTELTracerFromContext - return the trace.Tracer value from the context if it exists
func GetOTELTracerFromContext(ctx context.Context) (trace.Tracer, error) {
	v := ctx.Value(OpenTelemetryTracerCTXKey)
	if v == nil {
		// value not on context
		return nil, ErrNotInContext
	}
	if s, ok := v.(trace.Tracer); ok {
		return s, nil
	}
	// value not a string
	return nil, ErrValueWrongType
}

// GetOTELPropagatorsFromContext - return the trace.Propogators value from the context if it exists
func GetOTELPropagatorsFromContext(ctx context.Context) (propagation.TextMapPropagator, error) {
	v := ctx.Value(OpenTelemetryPropagatorsCTXKey)
	if v == nil {
		// value not on context
		return nil, ErrNotInContext
	}
	if s, ok := v.(propagation.TextMapPropagator); ok {
		return s, nil
	}
	// value not a string
	return nil, ErrValueWrongType
}

// GetOTELTracerPropagatorsFromContext - return the trace.Propogators value from the context if it exists
func GetOTELTracerPropagatorsFromContext(ctx context.Context, ns string) (trace.Tracer, propagation.TextMapPropagator, error) {
	// get or setup an opentelemetry tracer
	tracer, err := GetOTELTracerFromContext(ctx)
	if err != nil {
		if err != ErrNotInContext && err != ErrValueWrongType {
			// butsomething else is wrong
			return nil, nil, fmt.Errorf("unexpected err getting tracer from context: %w", err)
		}
		tracer = otel.Tracer(ns)
	}

	// get or setup propagator
	propagators, err := GetOTELPropagatorsFromContext(ctx)
	if err != nil {
		if err != ErrNotInContext && err != ErrValueWrongType {
			// something else is wrong
			return nil, nil, fmt.Errorf("unexpected err getting propagator from context: %w", err)
		}
		propagators = otel.GetTextMapPropagator()
	}
	return tracer, propagators, nil
}
