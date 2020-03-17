package errors

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
