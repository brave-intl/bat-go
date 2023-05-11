package backoff

import (
	"context"
	"errors"
	"testing"
	"time"

	mockretrypolicy "github.com/brave-intl/bat-go/libs/backoff/retrypolicy/mock"

	"github.com/brave-intl/bat-go/libs/backoff/retrypolicy"
	testutils "github.com/brave-intl/bat-go/libs/test"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestRetry_CxtDone(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx, done := context.WithCancel(context.Background())

	operation := func() (interface{}, error) {
		assert.Fail(t, "should not have been executed")
		return nil, nil
	}

	policy := mockretrypolicy.NewMockRetry(mockCtrl)

	isRetriable := func(error) bool {
		assert.Fail(t, "should not have been executed")
		return false
	}

	done()
	response, err := Retry(ctx, operation, policy, isRetriable)

	assert.Nil(t, response)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRetry_IsRetriable_False(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx, done := context.WithCancel(context.Background())
	defer done()

	expected := errors.New(testutils.RandomString())

	operation := func() (interface{}, error) {
		return nil, expected
	}

	policy := mockretrypolicy.NewMockRetry(mockCtrl)

	isRetriable := func(error) bool {
		return false
	}

	response, err := Retry(ctx, operation, policy, isRetriable)

	assert.Nil(t, response)
	assert.ErrorIs(t, err, expected)
}

func TestRetry_CalculateNextDelay_Done(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx, done := context.WithCancel(context.Background())
	defer done()

	expected := errors.New(testutils.RandomString())

	operation := func() (interface{}, error) {
		return nil, expected
	}

	policy := mockretrypolicy.NewMockRetry(mockCtrl)
	policy.EXPECT().
		CalculateNextDelay().
		Return(retrypolicy.Done)

	isRetriable := func(error) bool {
		return true
	}

	response, err := Retry(ctx, operation, policy, isRetriable)

	assert.Nil(t, response)
	assert.ErrorIs(t, err, expected)
}

func TestRetry(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx, done := context.WithCancel(context.Background())
	defer done()

	count := 0
	attempts := 2

	operation := func() (interface{}, error) {
		if count < attempts {
			count++
			return nil, errors.New(testutils.RandomString())
		}
		// return on third attempt
		return "success", nil
	}

	policy := mockretrypolicy.NewMockRetry(mockCtrl)
	policy.EXPECT().
		CalculateNextDelay().
		Return(time.Second * 0).
		Times(attempts)

	isRetriable := func(error) bool {
		return true
	}

	response, err := Retry(ctx, operation, policy, isRetriable)

	assert.Nil(t, err)
	assert.NotNil(t, response)
}
