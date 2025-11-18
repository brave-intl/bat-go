//go:build ratiosintegration

package ratios

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/brave-intl/bat-go/libs/clients"
	errorutils "github.com/brave-intl/bat-go/libs/errors"
	"github.com/stretchr/testify/assert"
)

func TestFetchRate(t *testing.T) {
	ctx := context.Background()

	client, err := New()
	assert.NoError(t, err, "Must be able to correctly initialize the client")

	resp, err := client.FetchRate(ctx, "unknown", "unknown")
	assert.Error(t, err, "should get an error back")
	var rateResponse *RateResponse
	assert.Equal(t, rateResponse, resp, "should be nil when error returned")

	var bundle *errorutils.ErrorBundle
	errors.As(err, &bundle)
	response, ok := bundle.Data().(clients.HTTPState)
	assert.Equal(t, true, ok)
	assert.Equal(t, http.StatusNotFound, response.Status)
}
