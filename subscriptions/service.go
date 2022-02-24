package subscriptions

import (
	"context"

	srv "github.com/brave-intl/bat-go/utils/service"
)

type Service struct {
	jobs      []srv.Job
	Datastore DataStore
}

// NewService - create a new rewards service structure
func NewService(ctx context.Context) *Service {
	return &Service{
		jobs: []srv.Job{},
	}
}

// InitService creates a service using the passed context
func InitService(ctx context.Context) (*Service, error) {
	// get logger from context
	// logger, err := appctx.GetLogger(ctx)
	// if err != nil {
	// 	ctx, logger = logging.SetupLogger(ctx)
	// }

	return NewService(ctx), nil
}
