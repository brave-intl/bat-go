package errors_test

import (
	"errors"
	"fmt"
	"testing"

	errutil "github.com/brave-intl/bat-go/utils/errors"
)

type customErr struct{}

func (ce *customErr) Error() string {
	return "custom error"
}

func TestMultiErrorUnwrap(t *testing.T) {
	var (
		err1b = errors.New("error 1b")
		err1a = fmt.Errorf("error 1a: %w", err1b)
		err1  = fmt.Errorf("error 1: %w", err1a)
		err2  = errors.New("error 2")
		err3  = &customErr{}
	)
	merr := &errutil.MultiError{}
	merr.Append(err1, err2, err3)

	var myCustomErr *customErr
	if !errors.As(merr, &myCustomErr) {
		t.Error("failed to unwrap multierror correctly: not 'as' err3")
	}

	if !errors.Is(merr, err1a) {
		t.Error("failed to unwrap multierror correctly: not 'is' err1a")
	}

	if !errors.Is(merr, err1b) {
		t.Error("failed to unwrap multierror correctly: not 'is' err1b")
	}

	if !errors.Is(merr, err1) {
		t.Error("failed to unwrap multierror correctly: not 'is' err1")
	}

	if !errors.Is(merr, err2) {
		t.Error("failed to unwrap multierror correctly: not 'is' err2")
	}
}
