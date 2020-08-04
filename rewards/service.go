package rewards

import (
	"context"
	"errors"
	"fmt"

	"github.com/brave-intl/bat-go/utils/clients/ratios"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/logging"
	srv "github.com/brave-intl/bat-go/utils/service"
)

// NewService - create a new rewards service structure
func NewService(ctx context.Context, ratio ratios.Client) *Service {
	return &Service{
		jobs:   []srv.Job{},
		ratios: ratio,
	}
}

// Service contains datastore
type Service struct {
	jobs []srv.Job
	// ratios client
	ratios ratios.Client
}

// Jobs - Implement srv.JobService interface
func (s *Service) Jobs() []srv.Job {
	return s.jobs
}

// InitService creates a service using the passed context
func InitService(ctx context.Context) (*Service, error) {
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

	return NewService(ctx, client), nil
}

// GetParameters - respond to caller with the rewards parameters
func (s *Service) GetParameters(ctx context.Context, currency *BaseCurrency) (*ParametersV1, error) {
	// get logger from context
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		ctx, logger = logging.SetupLogger(ctx)
	}

	rateData, err := s.ratios.FetchRate(ctx, "BAT", currency.String())
	if err != nil {
		logger.Error().Err(err).Msg("failed to fetch rate from ratios")
		return nil, fmt.Errorf("failed to fetch rate from ratios: %w", err)
	}
	if rateData == nil {
		logger.Error().Msg("empty response from ratios")
		return nil, errors.New("empty response from ratios")
	}

	var choices = getChoices(ctx, rateData.Payload[currency.String()])
	var defaultChoice float64
	if len(choices) > 1 {
		defaultChoice = choices[len(choices)/2]
	} else if len(choices) > 0 {
		defaultChoice = choices[0]
	}

	var rate, _ = rateData.Payload[currency.String()].Float64()

	return &ParametersV1{
		BATRate: rate,
		AutoContribute: AutoContribute{
			DefaultChoice: defaultChoice,
			Choices:       choices,
		},
		Tips: Tips{
			DefaultTipChoices:     getTipChoices(ctx),
			DefaultMonthlyChoices: getMonthlyChoices(ctx),
		},
	}, nil
}
