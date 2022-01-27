package errors

import (
	"errors"
	"fmt"
)

var (
	// ErrConflictBAPReportEvent is an error created when trying to update a bat loss event with a different amount
	ErrConflictBAPReportEvent = errors.New("unable to record BAP report")
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
	// ErrNotImplemented - this function is not yet implemented
	ErrNotImplemented = errors.New("this function is not yet implemented")
	// ErrNotFound - resource not found
	ErrNotFound = errors.New("not found")
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
func (err ErrorBundle) Data() interface{} {
	return err.data
}

// Cause returns the associated cause
func (err ErrorBundle) Cause() error {
	return err.cause
}

// Unwrap returns the associated cause
func (err ErrorBundle) Unwrap() error {
	return err.cause
}

// Error turns into an error
func (err ErrorBundle) Error() string {
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

type wErrs struct {
	err   error
	cause error
}

func (we *wErrs) Cause(err error) {
	we.cause = err
}

func (we *wErrs) Error() string {
	var result string
	if we.err != nil {
		result = we.err.Error()
	}
	if we.cause != nil {
		result += ": " + we.cause.Error()
	}
	return result
}

// Is - implement interface{ Is(error) bool } for equality check
func (we *wErrs) Is(err error) bool {
	if err == we.err {
		return true
	}
	return false
}

// As - implement interface{ As(target interface{}) bool } for equality check
func (we *wErrs) As(target interface{}) bool {
	return errors.As(we.err, target)
}

// Unwrap - implement unwrap interface to get the cause
func (we *wErrs) Unwrap() error {
	return we.cause
}

// Unwrap - implement Unwrap for unwrapping sub errors
func (me *MultiError) Unwrap() error {
	var errs []error
	// iterate over all the errors and wrapped errors
	// make a list so we can put them in wErr nodes
	for _, v := range me.Errs {
		vv := v
		for {
			errs = append(errs, vv)
			// unwrap until cant
			err := errors.Unwrap(vv)
			if err == nil {
				break
			}
			vv = err
		}
	}

	var wrappedErr = new(wErrs)
	for _, v := range errs {
		if v != nil {
			wrappedErr = &wErrs{err: v, cause: wrappedErr}
		}
	}
	wrappedErr = &wErrs{err: errors.New("wrapped errors"), cause: wrappedErr}

	return wrappedErr

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

// DrainCodified - Job runner drain codified errors have DrainCode()
type DrainCodified interface {
	// DrainCode - get the drain code from the interface implementation
	DrainCode() (string, bool)
}

// Codified - implementation of DrainCodified
type Codified struct {
	ErrCode string
	Retry   bool
}

// DrainCode - implementation of DrainCodified.DrainCode
func (c Codified) DrainCode() (string, bool) {
	return c.ErrCode, c.Retry
}
