package context

import (
	"context"
)

// wrapper allows for wrapping the values of a context with the cancellation of a new one
// approach from https://github.com/posener/ctxutil
type wrapper struct {
	wrapped context.Context
	context.Context
}

// Value returns the value associated with this context for key, or nil
// if no value is associated with key. Successive calls to Value with
// the same key returns the same result.
func (w *wrapper) Value(k interface{}) interface{} {
	if v := w.Context.Value(k); v != nil {
		return v
	}
	return w.wrapped.Value(k)
}

// Wrap a context, inheriting the values of the wrapped context
// nolint:golint
func Wrap(wrapped context.Context, context context.Context) context.Context {
	return &wrapper{wrapped, context}
}
