package subscriptions

import (
	"context"
	"fmt"

	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/logging"
	srv "github.com/brave-intl/bat-go/utils/service"
)

type Service struct {
	jobs      []srv.Job
	Datastore Datastore
	SKUClient SkuClient
}

// NewService - create a new subscriptions service structure
func NewService(ctx context.Context, datastore Datastore, skuClient SkuClient) *Service {
	return &Service{
		jobs:      []srv.Job{},
		Datastore: datastore,
		SKUClient: skuClient,
	}
}

// InitService creates a service using the passed context
func InitService(ctx context.Context) (*Service, error) {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		ctx, logger = logging.SetupLogger(ctx)
	}

	subscriptionPG, err := NewPostgres("", true, "subscriptions")
	if err != nil {
		logger.Panic().Err(err).Msg("Must be able to init postgres connection to start")
	}

	skuServiceAddress, err := appctx.GetStringFromContext(ctx, appctx.SKUsServerCTXKey)
	if err != nil {
		logger.Error().Err(err).Msg("failed to initialize sku client")
		return nil, fmt.Errorf("failed to initialize sku client: %w", err)
	}
	skuServiceToken, err := appctx.GetStringFromContext(ctx, appctx.SKUsTokenCTXKey)
	if err != nil {
		logger.Error().Err(err).Msg("failed to initialize sku client")
		return nil, fmt.Errorf("failed to initialize sku client: %w", err)
	}

	skuClient := InitSKUClient(skuServiceAddress, skuServiceToken)
	return NewService(ctx, subscriptionPG, *skuClient), nil
}

// Jobs - Implement srv.JobService interface
func (s *Service) Jobs() []srv.Job {
	return s.jobs
}
