package service

import (
	"context"
	"time"
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
