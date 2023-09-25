package payments

import (
	"fmt"
)

// PaymentError is an error used to communicate whether an error is temporary.
type PaymentError struct {
	OriginalError  error
	Temporary      bool
}

// Error makes ProcessingError an error
func (e PaymentError) Error() string {
	msg := fmt.Sprintf("error: %s", e.OriginalError)
	if e.Cause() != nil {
		msg = fmt.Sprintf("%s: %s", msg, e.Cause())
	}
	return msg
}

// Cause implements Cause for error
func (e PaymentError) Cause() error {
	return e.OriginalError
}

// Unwrap implements Unwrap for error
func (e PaymentError) Unwrap() error {
	return e.OriginalError
}

// ProcessingErrorFromError - given an error turn it into a processing error
func ProcessingErrorFromError(cause error, isTemporary bool) error {
	return &PaymentError{
		OriginalError:  cause,
		Temporary:      isTemporary,
	}
}
