package promotion

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnmarshalText(t *testing.T) {
	encoded := "eyJ0eXBlIjogImF1dG8tY29udHJpYnV0ZSIsICJjaGFubmVsIjogImJyYXZlLmNvbSJ9"
	var expected, d Suggestion
	expected.Type = "auto-contribute"
	expected.Channel = "brave.com"

	err := d.Base64Decode(encoded)
	assert.NoError(t, err, "Failed to unmarshal")
	assert.Equal(t, expected, d)
}
