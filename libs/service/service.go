package service

import (
	"context"
	"time"

	"github.com/brave-intl/bat-go/libs/clients"
	errorutils "github.com/brave-intl/bat-go/libs/errors"
	"github.com/brave-intl/bat-go/libs/logging"
	sentry "github.com/getsentry/sentry-go"
)

// JobFunc - type that defines what a Job Function should look like
type JobFunc func(context.Context) (bool, error)

// Job - Structure defining what a common job meta-information
type Job struct {
	Func    JobFunc
	Workers int
	Cadence time.Duration
}

// JobService - interface defining what can have jobs
type JobService interface {
	Jobs() []Job
}

// JobWorker - a job worker
func JobWorker(ctx context.Context, job func(context.Context) (bool, error), duration time.Duration) {
	logger := logging.Logger(ctx, "service.JobWorker")
	for {
		_, err := job(ctx)
		if err != nil {
			log := logger.Error().Err(err)
			httpError, ok := err.(*errorutils.ErrorBundle)
			if ok {
				state, ok := httpError.Data().(clients.HTTPState)
				if ok {
					log = log.Int("status", state.Status).
						Str("path", state.Path).
						Interface("data", state.Body)
				}
			}
			log.Msg("error encountered in job run")
			sentry.CaptureException(err)
		}
		// regardless if attempted or not, wait for the duration until retrying
		<-time.After(duration)
	}
}
