package rewards

import (
	"context"
	"errors"
	"fmt"

	"github.com/brave-intl/bat-go/utils/clients/ratios"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/logging"
	srv "github.com/brave-intl/bat-go/utils/service"
	"github.com/shopspring/decimal"
)

func init() {
	// remove the quotes in json marshaling
	decimal.MarshalJSONWithoutQuotes = true
}

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
func (s *Service) GetParameters(ctx context.Context, currency *RewardsBaseCurrency) (*Parameters, error) {
	// get logger from context
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		ctx, logger = logging.SetupLogger(ctx)
	}

	// get from ratios the current bat rate
	client, err := ratios.NewWithContext(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("failed to initialize the ratios client")
		return nil, fmt.Errorf("failed to initialize ratios client: %w", err)
	}

	rateData, err := client.FetchRate(ctx, "BAT", currency.String())
	if err != nil {
		return nil, fmt.Errorf("failed to fetch rate from ratios: %w", err)
	}
	if rateData == nil {
		return nil, errors.New("empty response from ratios")
	}

	return &Parameters{
		BATRate: rateData.Payload[currency.String()],
		AutoContribute: AutoContribute{
			Choices: getChoices(ctx, rateData.Payload[currency.String()]),
		},
		Tips: Tips{
			DefaultTipChoices:     getTipChoices(ctx),
			DefaultMonthlyChoices: getMonthlyChoices(ctx),
		},
	}, nil
}
