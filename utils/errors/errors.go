package errors

import (
	"fmt"
)

// Error interface
type Error interface {
	Error() string
	Cause() string
	Data() interface{}
}

// ErrorBundle creates a new response error
type ErrorBundle struct {
	cause string
	data  interface{}
}

// New creates a new response error
func New(cause string, data interface{}) error {
	return &ErrorBundle{
		cause,
		data,
	}
}

// Data from error origin
func (err *ErrorBundle) Data() interface{} {
	return err.data
}

// Cause returns the associated cause
func (err *ErrorBundle) Cause() string {
	return err.cause
}

// Error turns into an error
func (err *ErrorBundle) Error() string {
	return fmt.Sprintf("error: %s", err.cause)
}
