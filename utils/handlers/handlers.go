package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/asaskevich/govalidator"
	"github.com/getsentry/sentry-go"
	"github.com/rs/zerolog"
)

// AppError is error type for json HTTP responses
type AppError struct {
	Cause   error       `json:"-"`
	Message string      `json:"message"`
	Code    int         `json:"code"`
	Data    interface{} `json:"data,omitempty"`
}

// Error makes app error an error
func (e AppError) Error() string {
	msg := fmt.Sprintf("error: %s", e.Message)
	if e.Cause != nil {
		msg = fmt.Sprintf("%s: %s", msg, e.Cause)
	}
	return msg
}

// ServeHTTP responds according to the passed AppError
func (e AppError) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(e.Code)
	if err := json.NewEncoder(w).Encode(e); err != nil {
		panic(err)
	}
}

// WrapError with an additional message as an AppError
func WrapError(err error, msg string, passedCode int) *AppError {
	// FIXME err should probably be first
	appErr, ok := err.(AppError)
	if !ok {
		code := passedCode
		if code == 0 {
			code = http.StatusBadRequest
		}
		// use defaults passed in
		return &AppError{
			Cause:   err,
			Message: msg,
			Code:    code,
		}
	}
	code := appErr.Code
	if code == 0 {
		code = passedCode
	}
	if len(msg) != 0 {
		msg = fmt.Sprintf("%s: ", msg)
	}
	return &AppError{
		Cause:   appErr.Cause,
		Message: fmt.Sprintf("%s%s", msg, appErr.Message),
		Code:    code,
		Data:    appErr.Data,
	}
}

// WrapValidationError from govalidator
func WrapValidationError(err error) *AppError {
	return ValidationError("request body", govalidator.ErrorsByField(err))
}

// ValidationError creates an error to communicate a bad request was formed
func ValidationError(message string, validationErrors interface{}) *AppError {
	return &AppError{
		Message: "Error validating " + message,
		Code:    http.StatusBadRequest,
		Data: map[string]interface{}{
			"validationErrors": validationErrors,
		},
	}
}

// AppHandler is an http.Handler with JSON requests / reponses
type AppHandler func(http.ResponseWriter, *http.Request) *AppError

// ServeHTTP responds via the passed handler and handles returned errors
func (fn AppHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("content-type", "application/json")

	if e := fn(w, r); e != nil {
		if e.Code >= 500 && e.Code <= 599 {
			if e.Cause != nil {
				sentry.CaptureException(fmt.Errorf("%s: %w", e.Message, e.Cause))
			} else {
				sentry.CaptureMessage(e.Message)
			}
		}

		l := zerolog.Ctx(r.Context())
		l.UpdateContext(func(c zerolog.Context) zerolog.Context {
			return c.Err(e)
		})

		if e.Cause != nil {
			// Combine error with message
			e.Message = fmt.Sprintf("%s: %v", e.Message, e.Cause)
		}

		e.ServeHTTP(w, r)
	}
}
