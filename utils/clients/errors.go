package clients

import (
	"errors"
	"fmt"

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
// This return an &errors.ErrorBundle which wraps an HTTPState as its data field
func NewHTTPError(err error, path, message string, status int, v interface{}) error {
	return errorutils.New(err, message, HTTPState{
		Status: status,
		Path:   path,
		Body:   v,
	})
}

// ErrorData wraps the error data returned from an http request
type ErrorData struct {
	ResponseHeaders interface{} `json:"responseBody"`
	Body            interface{} `json:"body"`
}

// UnwrapErrorData this is a helper function to retrieve the wrapped ErrorData from HTTPState
func UnwrapErrorData(err error) (*ErrorData, error) {
	var errorBundle *errorutils.ErrorBundle
	if errors.As(err, &errorBundle) {
		if httpState, ok := errorBundle.Data().(HTTPState); ok {
			if errorData, ok := httpState.Body.(ErrorData); ok {
				return &errorData, nil
			}
		}
	}
	return nil, fmt.Errorf("error unwrapping error data for error %w", err)
}

// BitflyerError holds error info directly from bitflyer
type BitflyerError struct {
	Message        string   `json:"message"`
	ErrorIDs       []string `json:"errors"`
	Label          string   `json:"label"`
	Status         int      `json:"status"` // might be signed
	HTTPStatusCode int      `json:"-"`
}

// Error returns the error string
func (bfe *BitflyerError) Error() string {
	return fmt.Sprintf("message: %s - label: %s - status: %d - ids: %v - http status: %d", bfe.Message, bfe.Label, bfe.Status, bfe.ErrorIDs, bfe.HTTPStatusCode)
}
