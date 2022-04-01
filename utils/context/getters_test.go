package context

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetLogger(t *testing.T) {
	ctx := context.Background()
	actual, err := GetLogger(ctx)
	assert.Nil(t, actual)
	assert.EqualError(t, err, fmt.Sprintf("logger not found in context: %s", ErrNotInContext.Error()))
}
