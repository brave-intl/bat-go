package handlers

import (
	"errors"
	"net/http"
	"testing"
)

func TestWrapError(t *testing.T) {
	originatingError := errors.New("originating error")
	err := WrapError(originatingError, "wrapping message", http.StatusInternalServerError)
	if got, want := err.Code, http.StatusInternalServerError; got != want {
		t.Fatalf("AppError holds error code creation stable, got = %v, want = %v", got, want)
	}
	if got, want := err.Message, "wrapping message"; got != want {
		t.Fatalf("AppError holds error message creation stable, got = %v, want = %v", got, want)
	}
	if got, want := err.Cause, originatingError; got != want {
		t.Fatalf("AppError holds error pointer stable, got = %v, want = %v", got, want)
	}

	appErr := &AppError{
		Code: 1,
	}
	err = WrapError(appErr, "", 0)
	if got, want := err.Code, 1; got != want {
		t.Fatalf("AppError.Code should be original error value got %v, want %v", got, want)
	}

	appErr = &AppError{}
	err = WrapError(appErr, "", 5)
	if got, want := err.Code, 5; got != want {
		t.Fatalf("AppError.Code should be provided default value got %v, want %v", got, want)
	}

	appErr = &AppError{
		Message: "a",
	}
	err = WrapError(appErr, "b", 0)
	if got, want := err.Message, "b: a"; got != want {
		t.Fatalf("AppError.Message wraps error messages got %v, want %v", got, want)
	}
	if got, want := err.Error(), "error: b: a"; got != want {
		t.Fatalf("AppError.Message wraps error messages got %v, want %v", got, want)
	}
	err = WrapError(err, "c", 0)
	if got, want := err.Message, "c: b: a"; got != want {
		t.Fatalf("AppError.Error() wraps error messages recursively got %v, want %v", got, want)
	}
	if got, want := err.Error(), "error: c: b: a"; got != want {
		t.Fatalf("AppError.Error() wraps error messages recursively got %v, want %v", got, want)
	}

	appErr = &AppError{
		Message: "start",
		Cause:   errors.New("because"),
	}
	if got, want := appErr.Error(), "error: start: because"; got != want {
		t.Fatalf("AppError.Error() wraps error messages and appends the cause to the end got %v, want %v", got, want)
	}
	err = WrapError(nil, "does not have to be passed", 403)
	if got, want := err.Message, "does not have to be passed"; got != want {
		t.Fatalf("AppError does not need to be passed")
	}
	if got, want := err.Error(), "error: does not have to be passed"; got != want {
		t.Fatalf("AppError.Error() wraps error messages can stand alone got %v, want %v", got, want)
	}
}
