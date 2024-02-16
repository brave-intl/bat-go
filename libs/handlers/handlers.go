package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/libs/requestutils"
	"github.com/getsentry/sentry-go"
	"github.com/rs/zerolog"
)

// AppError is error type for json HTTP responses
type AppError struct {
	Cause     error       `json:"-"`
	Message   string      `json:"message"`             // description of failure
	ErrorCode string      `json:"errorCode,omitempty"` // short error code string
	Code      int         `json:"code"`                // status code for some reason
	Data      interface{} `json:"data,omitempty"`      // application specific data
}

// Error makes app error an error
func (e *AppError) Error() string {
	msg := "error: " + e.Message
	if e.Cause != nil {
		msg = msg + ": " + e.Cause.Error()
	}

	return msg
}

// ServeHTTP responds according to the passed AppError
func (e *AppError) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(e.Code)
	if err := json.NewEncoder(w).Encode(e); err != nil {
		panic(err)
	}
}

// WrapError with an additional message as an AppError
func WrapError(err error, msg string, passedCode int) *AppError {
	// FIXME err should probably be first
	// appErr, ok := err.(*AppError)
	var appErr *AppError
	if !errors.As(err, &appErr) {
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

// RenderContent based on the header
func RenderContent(ctx context.Context, v interface{}, w http.ResponseWriter, status int) *AppError {
	switch w.Header().Get("content-type") {
	case "application/json":
		var b bytes.Buffer

		if err := json.NewEncoder(&b).Encode(v); err != nil {
			return WrapError(err, "Error encoding JSON", http.StatusInternalServerError)
		}

		w.WriteHeader(status)
		_, err := w.Write(b.Bytes())
		// Should never happen :fingers_crossed:
		if err != nil {
			return WrapError(err, "Error writing a response", http.StatusInternalServerError)
		}
	}

	return nil
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

// AppHandler is an http.Handler with JSON requests / responses
type AppHandler func(http.ResponseWriter, *http.Request) *AppError

// ServeHTTP responds via the passed handler and handles returned errors
func (fn AppHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.Contains(r.Header.Get("Accept"), "application/json") ||
		strings.Contains(r.Header.Get("Accept"), "*/*") || r.Header.Get("Accept") == "" {
		w.Header().Set("content-type", "application/json")
	} else {
		w.WriteHeader(http.StatusBadRequest)
		// return a 400 error here as we cannot supply the encoding type the client is asking for
	}

	if e := fn(w, r); e != nil {
		if e.Code >= 500 && e.Code <= 599 {
			sentry.WithScope(func(scope *sentry.Scope) {
				scope.SetTags(map[string]string{
					"reqID": requestutils.GetRequestID(r.Context()),
				})
				sentry.CaptureException(e)
			})
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
