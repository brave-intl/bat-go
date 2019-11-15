package clients

import (
	"github.com/brave-intl/bat-go/utils/errors"
)

// HTTPState captures the state of the response to be read by lower fns in the stack
type HTTPState struct {
	Status int
	Body   interface{}
}

// NewHTTPError creates a new response state
func NewHTTPError(cause string, status int, v interface{}) error {
	return errors.New(cause, HTTPState{
		Status: status,
		Body:   v,
	})
}
