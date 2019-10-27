// +build integration

package ads

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetAdsCountries(t *testing.T) {
	ctx := context.Background()

	client, err := New()
	assert.NoError(t, err, "Must be able to correctly initialize the client")

	countries, err := client.GetAdsCountries(ctx)
	assert.NoError(t, err, "Should be able to get countries where ads are available")
	_, present := countries["US"]
	assert.True(t, present, "US should be in the list of countries")
}
