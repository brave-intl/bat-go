package retrypolicy

// Convenience retry policies

import "time"

var (
	// DefaultRetry a default policy
	DefaultRetry, _ = New(
		WithInitialInterval(50*time.Millisecond),
		WithBackoffCoefficient(2.0),
		WithMaximumInterval(10*time.Second),
		WithExpirationInterval(time.Minute),
		WithMaximumAttempts(10),
	)

	// NoRetry policy to be used if no retries are required
	NoRetry, _ = New(
		WithInitialInterval(0),
		WithBackoffCoefficient(0),
		WithMaximumInterval(0),
		WithExpirationInterval(0),
		WithMaximumAttempts(0),
	)
)
