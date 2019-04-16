package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/asaskevich/govalidator"
	raven "github.com/getsentry/raven-go"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
)

// AppError is error type for json HTTP responses
type AppError struct {
	Error   error                  `json:"-"`
	Message string                 `json:"message"`
	Code    int                    `json:"code"`
	Data    map[string]interface{} `json:"data,omitempty"`
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
func WrapError(err error, msg string, code int) *AppError {
	// FIXME err should probably be first
	if code == 0 {
		code = http.StatusBadRequest
	}
	return &AppError{
		Error:   err,
		Message: msg,
		Code:    code,
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
			if e.Error != nil {
				raven.CaptureError(errors.Wrap(e.Error, e.Message), map[string]string{})
			} else {
				raven.CaptureMessage(e.Message, map[string]string{})
			}
		}

		if e.Error != nil {
			// Combine error with message
			e.Message = fmt.Sprintf("%s: %v", e.Message, e.Error)
		}
		ctx := log.Logger.WithContext(r.Context())
		log.Ctx(ctx).Error().Msg(e.Message)

		e.ServeHTTP(w, r)
	}
}
