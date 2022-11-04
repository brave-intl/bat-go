package errors

import (
	"encoding/json"
	"errors"
	"fmt"
)

var (
	// ErrInvalidCountry - invalid country error for validation
	ErrInvalidCountry = errors.New("invalid country")
	// ErrNoIdentityCountry - no specified identity country
	ErrNoIdentityCountry = errors.New("no identity country")
	// ErrConflictBAPReportEvent is an error created when trying to update a bat loss event with a different amount
	ErrConflictBAPReportEvent = errors.New("unable to record BAP report")
	// ErrConflictBATLossEvent is an error created when trying to update a bat loss event with a different amount
	ErrConflictBATLossEvent = errors.New("unable to update bat loss events")
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
	// ErrInternalServerError internal server error
	ErrInternalServerError = errors.New("server encountered an internal error and was unable to complete the request")
	// ErrBadRequest bad request error
	ErrBadRequest = errors.New("error bad request")
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
func (e ErrorBundle) Data() interface{} {
	return e.data
}

// Cause returns the associated cause
func (e ErrorBundle) Cause() error {
	return e.cause
}

// Unwrap returns the associated cause
func (e ErrorBundle) Unwrap() error {
	return e.cause
}

// Error turns into an error
func (e ErrorBundle) Error() string {
	return e.message
}

// DataToString returns string representation of data
func (e ErrorBundle) DataToString() string {
	if e.data == nil {
		return "no error bundle data"
	}
	b, err := json.Marshal(e.data)
	if err != nil {
		return fmt.Sprintf("error retrieving error bundle data %s", err.Error())
	}
	return string(b)
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
	return err == we.err
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
