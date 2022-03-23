package clients

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	testutils "github.com/brave-intl/bat-go/utils/test"
	"github.com/stretchr/testify/assert"
)

func TestUnwrapErrorData(t *testing.T) {
	expected := ErrorData{
		ResponseHeaders: testutils.RandomString(),
		Body:            testutils.RandomString(),
	}

	err := errors.New(testutils.RandomString())
	errorBundle := NewHTTPError(err, testutils.RandomString(), err.Error(), http.StatusInternalServerError, expected)

	actual, err := UnwrapErrorData(errorBundle)
	assert.NoError(t, err)

	assert.Equal(t, &expected, actual)
}

func TestUnwrapErrorData_Error(t *testing.T) {
	err := errors.New(testutils.RandomString())
	errorData, actual := UnwrapErrorData(err)
	assert.Nil(t, errorData)
	assert.EqualError(t, actual, fmt.Errorf("error unwrapping error data for error %w", err).Error())
}
