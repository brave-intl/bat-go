package retrypolicy

import (
	"testing"
	"time"

	testutils "github.com/brave-intl/bat-go/libs/test"
	"github.com/stretchr/testify/assert"
)

func TestRetryPolicy_New(t *testing.T) {
	t.Parallel()
	initialInterval := time.Second
	backoffCoefficient := float64(testutils.RandomInt())
	maximumInterval := time.Second
	expirationInterval := time.Second
	maximumAttempts := testutils.RandomInt()

	retryPolicy, err := New(
		WithInitialInterval(initialInterval),
		WithBackoffCoefficient(backoffCoefficient),
		WithMaximumInterval(maximumInterval),
		WithExpirationInterval(expirationInterval),
		WithMaximumAttempts(maximumAttempts),
	)

	assert.NoError(t, err)
	assert.NotNil(t, retryPolicy)
}

func TestRetryPolicy_CalculateNextDelay_MaxAttempts(t *testing.T) {
	t.Parallel()
	retryPolicy := policy{
		currentAttempt: 1,
		maximumAttempt: 1,
	}
	assert.Equal(t, Done, retryPolicy.CalculateNextDelay())
}

func TestPolicy_CalculateNextDelay_ElapsedTimeGreaterThanExpirationInterval(t *testing.T) {
	t.Parallel()
	retryPolicy := policy{
		currentAttempt:     0,
		maximumAttempt:     10,
		expirationInterval: time.Second * 10,
		startTime:          time.Now().Add(-time.Second * 11),
	}
	assert.Equal(t, Done, retryPolicy.CalculateNextDelay())
}

func TestPolicy_CalculateNextDelay_NextIntervalIsZero(t *testing.T) {
	t.Parallel()
	retryPolicy := policy{
		currentAttempt:     0,
		maximumAttempt:     1,
		expirationInterval: time.Second * 10,
		startTime:          time.Now(),
		initialInterval:    0,
	}
	assert.Equal(t, Done, retryPolicy.CalculateNextDelay())
}

func TestPolicy_CalculateNextDelay_Default(t *testing.T) {
	t.Parallel()

	durations := []time.Duration{
		50 * time.Millisecond,
		100 * time.Millisecond,
		200 * time.Millisecond,
		400 * time.Millisecond,
		800 * time.Millisecond,
		1600 * time.Millisecond,
		3200 * time.Millisecond,
		6400 * time.Millisecond,
		10000 * time.Millisecond,
		// maximum default policy interval is 10 sec we should not exceed this value
		10000 * time.Millisecond,
		Done,
	}

	for _, expected := range durations {
		actual := DefaultRetry.CalculateNextDelay()

		if expected == Done {
			assert.Equal(t, Done, actual)
			break
		}

		// calculate minimumDuration to account for jitter
		minimumDuration := time.Duration(0.8 * float64(expected))
		assert.GreaterOrEqual(t, actual, minimumDuration)

		time.Sleep(actual)
	}
}

func TestPolicy_CalculateNextDelay_NoRetry(t *testing.T) {
	t.Parallel()
	assert.Equal(t, Done, NoRetry.CalculateNextDelay())
	assert.Equal(t, Done, NoRetry.CalculateNextDelay())
}
