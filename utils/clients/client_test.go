package clients

import (
	"context"
	"github.com/brave-intl/bat-go/utils/errors"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDo_ResponseBody_HttpState(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Length", "1")
	}))
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL, nil)
	assert.NoError(t, err)

	client, err := New(ts.URL, "")
	assert.NoError(t, err)

	var data *string
	response, err := client.Do(context.Background(), req, data)

	assert.IsType(t, err, &errors.ErrorBundle{})
	assert.NotNil(t, response)

	actual := err.(*errors.ErrorBundle)
	assert.Equal(t, "response", actual.Error())
	assert.NotNil(t, actual.Cause(), ErrUnableToDecode)

	httpState := actual.Data().(HTTPState)
	assert.Equal(t, httpState.Status, http.StatusOK)
	assert.Equal(t, ts.URL, httpState.Path)
	assert.NotEmpty(t, httpState.Body)
}
