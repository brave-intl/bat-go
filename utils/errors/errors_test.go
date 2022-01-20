package errors_test

import (
	"errors"
	"fmt"
	"testing"

	errutil "github.com/brave-intl/bat-go/utils/errors"
)

func TestMultiErrorUnwrap(t *testing.T) {
	var (
		err1b = errors.New("error 1b")
		err1a = fmt.Errorf("error 1a: %w", err1b)
		err1  = fmt.Errorf("error 1: %w", err1a)
		err2  = errors.New("error 2")
	)
	merr := &errutil.MultiError{}
	merr.Append(err1, err2)

	if !errors.Is(merr, err2) {
		t.Error("failed to unwrap multierror correctly")
	}
}
