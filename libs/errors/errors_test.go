package errors_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	testutils "github.com/brave-intl/bat-go/libs/test"
	"github.com/stretchr/testify/assert"

	errutil "github.com/brave-intl/bat-go/libs/errors"
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

func TestErrorBundle_DataToString_DataNil(t *testing.T) {
	err := errutil.Wrap(errors.New(testutils.RandomString()), testutils.RandomString())
	var actual *errutil.ErrorBundle
	errors.As(err, &actual)
	assert.Equal(t, "no error bundle data", actual.DataToString())
}

func TestErrorBundle_DataToString_MarshallError(t *testing.T) {
	unsupportedData := func() {}
	sut := errutil.New(errors.New(testutils.RandomString()), testutils.RandomString(), unsupportedData)

	expected := "error retrieving error bundle data"

	var actual *errutil.ErrorBundle
	errors.As(sut, &actual)

	assert.Contains(t, actual.DataToString(), expected)
}

func TestErrorBundle_DataToString(t *testing.T) {
	errorData := testutils.RandomString()
	sut := errutil.New(errors.New(testutils.RandomString()), testutils.RandomString(), errorData)

	expected, err := json.Marshal(errorData)
	assert.NoError(t, err)

	var actual *errutil.ErrorBundle
	errors.As(sut, &actual)

	assert.Equal(t, string(expected), actual.DataToString())
}
