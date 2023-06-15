package clients

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	testutils "github.com/brave-intl/bat-go/libs/test"
	"github.com/stretchr/testify/assert"
)

func TestUnwrapHTTPState(t *testing.T) {
	h := make(http.Header)
	h.Add(testutils.RandomString(), testutils.RandomString())

	expected := HTTPState{
		Status: http.StatusInternalServerError,
		Path:   testutils.RandomString(),
		Body: RespErrData{
			ResponseHeaders: h,
			Body:            testutils.RandomString(),
		},
	}

	err := errors.New(testutils.RandomString())
	errorBundle := NewHTTPError(err, expected.Path, err.Error(), expected.Status, expected.Body)

	actual, err := UnwrapHTTPState(errorBundle)
	assert.NoError(t, err)

	assert.Equal(t, &expected, actual)
}

func TestUnwrapHTTPState_Error(t *testing.T) {
	err := errors.New(testutils.RandomString())
	errorData, actual := UnwrapHTTPState(err)
	assert.Nil(t, errorData)
	assert.EqualError(t, actual, fmt.Errorf("error unwrapping http state for error %w", err).Error())
}
