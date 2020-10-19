package clients

import (
	errorutils "github.com/brave-intl/bat-go/utils/errors"
)

var (
	// ErrUnableToDecode unable to decode body
	ErrUnableToDecode = "unable to decode response"
	// ErrProtocolError the error was within the data that went into the endpoint
	ErrProtocolError = "protocol error"
	// ErrUnableToEscapeURL the url could nto be escaped
	ErrUnableToEscapeURL = "unable to escape url"
	// ErrInvalidHost the host was invalid
	ErrInvalidHost = "invalid host"
	// ErrMalformedRequest the request was malformed
	ErrMalformedRequest = "malformed request"
	// ErrUnableToEncodeBody body could not be decoded
	ErrUnableToEncodeBody = "unable to encode body"
)

// HTTPState captures the state of the response to be read by lower fns in the stack
type HTTPState struct {
	Status int
	Path   string
	Body   interface{}
}

// NewHTTPError creates a new response state
func NewHTTPError(err error, path, message string, status int, v interface{}) error {
	return errorutils.New(err, message, HTTPState{
		Status: status,
		Path:   path,
		Body:   v,
	})
}
