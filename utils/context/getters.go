package context

import "context"

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
