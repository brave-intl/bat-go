package retrypolicy

import (
	"crypto/rand"
	"fmt"
	"math"
	"math/big"
	"time"
)

// Done is returned when CalculateNextDelay has reached it delay retry limit
const Done time.Duration = -1

type (
	// Retry api
	Retry interface {
		// CalculateNextDelay implementations should return the next delay interval
		CalculateNextDelay() time.Duration
	}

	policy struct {
		startTime          time.Time
		currentAttempt     int
		initialInterval    time.Duration
		backoffCoefficient float64
		maximumInterval    time.Duration
		expirationInterval time.Duration
		maximumAttempt     int
	}

	// Option func to build retry policy
	Option func(policy *policy) error
)

// New return a new instance of retry policy
func New(options ...Option) (Retry, error) {
	retryPolicy := new(policy)

	retryPolicy.startTime = time.Now()
	retryPolicy.currentAttempt = 0

	for _, option := range options {
		if err := option(retryPolicy); err != nil {
			return nil, fmt.Errorf("error initializing retry policy %w", err)
		}
	}

	return retryPolicy, nil
}

// CalculateNextDelay returns the next delay interval based on the retry policy
func (p *policy) CalculateNextDelay() time.Duration {

	if p.currentAttempt >= p.maximumAttempt {
		return Done
	}

	elapsedTime := time.Since(p.startTime)

	if elapsedTime >= p.expirationInterval {
		return Done
	}

	nextInterval := float64(p.initialInterval) * math.Pow(p.backoffCoefficient, float64(p.currentAttempt))
	if nextInterval <= 0 {
		return Done
	}

	if p.maximumInterval != 0 {
		nextInterval = math.Min(nextInterval, float64(p.maximumInterval))
	}

	if p.expirationInterval != 0 {
		remainingTime := math.Max(0, float64(p.expirationInterval-elapsedTime))
		nextInterval = math.Min(remainingTime, nextInterval)
	}

	nextDuration := time.Duration(nextInterval)
	if nextDuration < p.initialInterval {
		return Done
	}

	jitter := int64(0.2 * nextInterval)
	if jitter < 1 {
		jitter = 1
	}

	n, err := rand.Int(rand.Reader, big.NewInt(jitter))
	if err != nil || n == nil {
		panic("panic generating random int for jitter")
	}
	nextInterval = nextInterval*0.8 + float64(n.Int64())

	p.currentAttempt++
	return time.Duration(nextInterval)
}

// WithInitialInterval sets the initial interval
func WithInitialInterval(initialInterval time.Duration) Option {
	return func(p *policy) error {
		p.initialInterval = initialInterval
		return nil
	}
}

// WithBackoffCoefficient sets the coefficient used to calculate next interval
func WithBackoffCoefficient(backoffCoefficient float64) Option {
	return func(p *policy) error {
		p.backoffCoefficient = backoffCoefficient
		return nil
	}
}

// WithMaximumInterval sets the maximum time that can be calculated for next interval
func WithMaximumInterval(maximumInterval time.Duration) Option {
	return func(p *policy) error {
		p.maximumInterval = maximumInterval
		return nil
	}
}

// WithExpirationInterval sets the maximum elapsed time an operation should be tried for
func WithExpirationInterval(expirationInterval time.Duration) Option {
	return func(p *policy) error {
		p.expirationInterval = expirationInterval
		return nil
	}
}

// WithMaximumAttempts sets the maximum number of times an operation will be tried
func WithMaximumAttempts(maximumAttempts int) Option {
	return func(p *policy) error {
		p.maximumAttempt = maximumAttempts
		return nil
	}
}
