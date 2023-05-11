package clients

import (
	"context"
	"fmt"
	"github.com/brave-intl/bat-go/libs/errors"
	testutils "github.com/brave-intl/bat-go/libs/test"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDo_ErrorWithResponse(t *testing.T) {
	errorMsg := testutils.RandomString()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(errorMsg))
		assert.NoError(t, err)
	}))
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL, nil)
	assert.NoError(t, err)

	client, err := New(ts.URL, "")
	assert.NoError(t, err)

	// pass data as invalid result type to cause error
	var data *string
	response, err := client.Do(context.Background(), req, data)

	assert.IsType(t, &errors.ErrorBundle{}, err)
	assert.NotNil(t, response)

	actual := err.(*errors.ErrorBundle)
	assert.Equal(t, "response", actual.Error())
	assert.NotNil(t, actual.Cause(), ErrUnableToDecode)

	httpState := actual.Data().(HTTPState)
	assert.Equal(t, httpState.Status, http.StatusOK)
	assert.Equal(t, ts.URL, httpState.Path)
	assert.Contains(t, fmt.Sprintf("+%v", httpState.Body), errorMsg)
}
