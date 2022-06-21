package rewards

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/asaskevich/govalidator"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/brave-intl/bat-go/utils/clients/ratios"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/logging"
	srv "github.com/brave-intl/bat-go/utils/service"
)

// PayoutStatus - current state of the payout status
type PayoutStatus struct {
	Unverified string `json:"unverified" valid:"in(off|processing|complete)"`
	Uphold     string `json:"uphold" valid:"in(off|processing|complete)"`
	Gemini     string `json:"gemini" valid:"in(off|processing|complete)"`
	Bitflyer   string `json:"bitflyer" valid:"in(off|processing|complete)"`
}

// HandleErrors - handle any errors in input
func (ps *PayoutStatus) HandleErrors(err error) *handlers.AppError {
	return handlers.ValidationError("invalid payout status", err)
}

// Decode - implement decodable
func (ps *PayoutStatus) Decode(ctx context.Context, input []byte) error {
	return json.Unmarshal(input, ps)
}

// Validate - implement validatable
func (ps *PayoutStatus) Validate(ctx context.Context) error {
	isValid, err := govalidator.ValidateStruct(ps)
	if err != nil {
		return err
	}
	if !isValid {
		return errors.New("invalid input")
	}
	return nil
}

// SetPayoutStatus - set the payout status state
func (s *Service) SetPayoutStatus(v *PayoutStatus) {
	s.payoutStatusMutex.Lock()
	s.payoutStatus = v
	defer s.payoutStatusMutex.Unlock()
}

// NewService - create a new rewards service structure
func NewService(ctx context.Context, ratio ratios.Client) (*Service, error) {
	// get the aws region from ctx
	region, ok := ctx.Value(appctx.AWSRegionCTXKey).(string)
	if !ok {
		return nil, errors.New("aws region is not configured")
	}

	// aws config
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("failed to load aws config: %w", err)
	}

	return &Service{
		jobs:     []srv.Job{},
		ratios:   ratio,
		s3Client: s3.FromConfig(cfg),
	}, nil
}

// Service contains datastore
type Service struct {
	jobs []srv.Job
	// ratios client
	ratios            ratios.Client
	payoutStatusMutex *sync.RWMutex
	payoutStatus      *PayoutStatus
	s3Client          *s3.Client
}

// Jobs - Implement srv.JobService interface
func (s *Service) Jobs() []srv.Job {
	return s.jobs
}

// InitService creates a service using the passed context
func InitService(ctx context.Context) (*Service, error) {
	// get logger from context
	logger := logging.Logger(ctx, "rewards.InitService")

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
	if currency == nil {
		currency = new(BaseCurrency)
		*currency = "usd"
	}

	var currencyStr = strings.ToLower(currency.String())
	// get logger from context
	logger := logging.Logger(ctx, "rewards.GetParameters")

	rateData, err := s.ratios.FetchRate(ctx, "bat", currencyStr)
	if err != nil {
		logger.Error().Err(err).Msg("failed to fetch rate from ratios")
		return nil, fmt.Errorf("failed to fetch rate from ratios: %w", err)
	}
	if rateData == nil {
		logger.Error().Msg("empty response from ratios")
		return nil, errors.New("empty response from ratios")
	}

	var choices = getChoices(ctx, rateData.Payload[currencyStr])
	var defaultChoice float64
	if len(choices) > 1 {
		defaultChoice = choices[len(choices)/2]
	} else if len(choices) > 0 {
		defaultChoice = choices[0]
	}

	// if there is a default choice configured use it
	if dc := getDefaultChoice(ctx); dc > 0 {
		defaultChoice = dc
	}

	var rate, _ = rateData.Payload[currencyStr].Float64()

	s.payoutStatusMutex.RLock()
	defer s.payoutStatusMutex.RUnlock()

	return &ParametersV1{
		PayoutStatus: s.payoutStatus,
		BATRate:      rate,
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
