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

	operation := func() (any, error) {
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

	ctx := t.Context()

	expected := errors.New(testutils.RandomString())

	operation := func() (any, error) {
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

	ctx := t.Context()

	expected := errors.New(testutils.RandomString())

	operation := func() (any, error) {
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

	ctx := t.Context()

	count := 0
	attempts := 2

	operation := func() (any, error) {
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

func TestRetry_BackoffDelays(t *testing.T) {
	ctx := t.Context()

	initialInterval := 10 * time.Millisecond
	policy, err := retrypolicy.New(
		retrypolicy.WithInitialInterval(initialInterval),
		retrypolicy.WithBackoffCoefficient(2.0),
		retrypolicy.WithExpirationInterval(time.Minute),
		retrypolicy.WithMaximumAttempts(3),
	)
	assert.NoError(t, err)

	count := 0
	attempts := 3
	timestamps := make([]time.Time, 0, attempts)

	operation := func() (any, error) {
		timestamps = append(timestamps, time.Now())
		if count < attempts-1 {
			count++
			return nil, errors.New(testutils.RandomString())
		}
		return "success", nil
	}

	response, err := Retry(ctx, operation, policy, func(error) bool { return true })

	assert.NoError(t, err)
	assert.Equal(t, "success", response)
	assert.Len(t, timestamps, attempts)

	firstDelay := timestamps[1].Sub(timestamps[0])
	secondDelay := timestamps[2].Sub(timestamps[1])
	assert.GreaterOrEqual(t, firstDelay, time.Duration(0.8*float64(initialInterval)))
	assert.GreaterOrEqual(t, secondDelay, 2*time.Duration(0.8*float64(initialInterval)))
	assert.Greater(t, secondDelay, firstDelay)
}

func TestRetry_ExhaustsAttempts(t *testing.T) {
	ctx := t.Context()

	expected := errors.New(testutils.RandomString())

	policy, err := retrypolicy.New(
		retrypolicy.WithInitialInterval(time.Millisecond),
		retrypolicy.WithBackoffCoefficient(1.0),
		retrypolicy.WithExpirationInterval(time.Minute),
		retrypolicy.WithMaximumAttempts(2),
	)
	assert.NoError(t, err)

	calls := 0
	operation := func() (any, error) {
		calls++
		return nil, expected
	}

	response, err := Retry(ctx, operation, policy, func(error) bool { return true })

	assert.Nil(t, response)
	assert.ErrorIs(t, err, expected)
	assert.Equal(t, 3, calls)
}
