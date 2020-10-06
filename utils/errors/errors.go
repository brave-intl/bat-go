package errors

import (
	"errors"
	"fmt"
)

var (
	// ErrConflictBATLossEvent is an error created when trying to update a bat loss event with a different amount
	ErrConflictBATLossEvent = errors.New("unable to update bat loss events")
	// ErrWalletNotFound when there is no wallet found
	ErrWalletNotFound = errors.New("unable to find wallet")
	// ErrCertificateExpired - a certificate is expired
	ErrCertificateExpired = errors.New("certificate expired")
	// ErrMarshalTransferRequest - failed to marshal the transfer request
	ErrMarshalTransferRequest = errors.New("failed to marshal the transfer request")
	// ErrCreateTransferRequest - failed to create the transfer request
	ErrCreateTransferRequest = errors.New("failed to create the transfer request")
	// ErrSignTransferRequest - failed to sign the transfer request
	ErrSignTransferRequest = errors.New("failed to sign the transfer request")
	// ErrFailedClientRequest - failed to perform client request
	ErrFailedClientRequest = errors.New("failed to perform api request")
	// ErrFailedBodyRead - failed to read body
	ErrFailedBodyRead = errors.New("failed to read the transfer response")
	// ErrFailedBodyUnmarshal - failed to decode body
	ErrFailedBodyUnmarshal = errors.New("failed to unmarshal the transfer response")
	// ErrMissingWallet - missing wallet
	ErrMissingWallet = errors.New("missing wallet")
	// ErrNoDepositProviderDestination - no linked wallet
	ErrNoDepositProviderDestination = errors.New("no deposit provider destination for wallet for transfer")
)

// ErrorBundle creates a new response error
type ErrorBundle struct {
	cause   error
	message string
	data    interface{}
}

// New creates a new response error
func New(cause error, message string, data interface{}) error {
	return &ErrorBundle{
		cause,
		message,
		data,
	}
}

// Data from error origin
func (err *ErrorBundle) Data() interface{} {
	return err.data
}

// Cause returns the associated cause
func (err *ErrorBundle) Cause() error {
	return err.cause
}

// Unwrap returns the associated cause
func (err *ErrorBundle) Unwrap() error {
	return err.cause
}

// Error turns into an error
func (err *ErrorBundle) Error() string {
	return err.message
}

// Wrap wraps an error
func Wrap(cause error, message string) error {
	return &ErrorBundle{
		cause:   cause,
		message: message,
		data:    nil,
	}
}

// MultiError - allows for multiple errors, not necessarily chained
type MultiError struct {
	Errs []error
}

// Append - append new errors to this multierror
func (me *MultiError) Append(err ...error) {
	if me.Errs == nil {
		me.Errs = []error{}
	}
	me.Errs = append(me.Errs, err...)
}

// Count - get the number of errors contained herein
func (me *MultiError) Count() int {
	return len(me.Errs)
}

// Error - implement Error interface
func (me *MultiError) Error() string {
	var errText string
	for _, err := range me.Errs {
		if errText == "" {
			errText = fmt.Sprintf("%s", err)
		} else {
			errText += fmt.Sprintf("; %s", err)
		}
	}
	return errText
}
