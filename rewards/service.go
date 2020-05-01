package rewards

import (
	"context"

	srv "github.com/brave-intl/bat-go/utils/service"
)

// NewService - create a new rewards service structure
func NewService(ctx context.Context) *Service {
	return &Service{
		jobs: []srv.Job{},
	}
}

// Service contains datastore
type Service struct {
	jobs []srv.Job
}

// Jobs - Implement srv.JobService interface
func (s *Service) Jobs() []srv.Job {
	return s.jobs
}

// InitService creates a service using the passed context
func InitService(ctx context.Context) (*Service, error) {
	return NewService(ctx), nil
}

// GetParameters - respond to caller with the rewards parameters
func (s *Service) GetParameters(ctx context.Context) (*Parameters, error) {
	var parameters = new(Parameters)

	// ... do things

	return parameters, nil
}
