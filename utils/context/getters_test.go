package context

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestGetLogger(t *testing.T) {
	ctx := context.Background()
	actual, err := GetLogger(ctx)
	assert.Nil(t, actual)
	assert.EqualError(t, err, fmt.Sprintf("logger not found in context: %s", ErrNotInContext.Error()))
}
