package backoff

import (
	"context"
	"time"

	"github.com/brave-intl/bat-go/libs/backoff/retrypolicy"
)

type (
	// RetryFunc defines a retry function
	RetryFunc func(ctx context.Context, operation Operation, retryPolicy retrypolicy.Retry, IsRetriable IsRetriable) (any, error)

	// Operation the operation to be executed with retry
	Operation func() (any, error)

	// IsRetriable a function to determine if an error caused by the executed operation is retriable
	IsRetriable func(error) bool
)

// Retry executes the given Operation using the provided retrypolicy.Retry policy and IsRetriable conditions
func Retry(ctx context.Context, operation Operation, retryPolicy retrypolicy.Retry, IsRetriable IsRetriable) (any, error) {

	var err error
	var response any
	var next time.Duration

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:

			if response, err = operation(); err == nil {
				return response, nil
			}

			if !IsRetriable(err) {
				return nil, err
			}

			if next = retryPolicy.CalculateNextDelay(); next == retrypolicy.Done {
				return nil, err
			}

			time.Sleep(next)
		}
	}
}
