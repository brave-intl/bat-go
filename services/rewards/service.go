package rewards

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	appaws "github.com/brave-intl/bat-go/libs/aws"
	"github.com/brave-intl/bat-go/libs/clients/ratios"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/custodian"
	"github.com/brave-intl/bat-go/libs/logging"
	srv "github.com/brave-intl/bat-go/libs/service"
)

// NewService - create a new rewards service structure
func NewService(ctx context.Context, ratio ratios.Client, s3client appaws.S3GetObjectAPI) (*Service, error) {
	logger := logging.Logger(ctx, "rewards.NewService")

	cfg, err := appaws.BaseAWSConfig(ctx, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create base aws config: %w", err)
	}

	logger.Info().Msg("checking s3 client")
	if s3client == nil {
		s3client = s3.NewFromConfig(cfg)
	}

	logger.Info().Str("s3client", fmt.Sprintf("%+v", s3client)).Msg("setup s3 client")
	return &Service{
		cacheMu:  new(sync.RWMutex),
		jobs:     []srv.Job{},
		ratios:   ratio,
		s3Client: s3client,
	}, nil
}

// Service contains datastore
type Service struct {
	lastPollTime         time.Time
	lastPayoutStatus     *custodian.PayoutStatus
	lastCustodianRegions *custodian.Regions
	cacheMu              *sync.RWMutex

	jobs []srv.Job
	// ratios client
	ratios   ratios.Client
	s3Client appaws.S3GetObjectAPI
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

	logger.Info().Msg("creating new rewards parameters service")

	return NewService(ctx, client, nil)
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

	s.cacheMu.RLock()
	lastPollTime := s.lastPollTime
	s.cacheMu.RUnlock()

	if time.Now().After(lastPollTime.Add(15 * time.Minute)) {
		// merge in static s3 attributes into response
		var (
			payoutStatus     *custodian.PayoutStatus
			custodianRegions *custodian.Regions
			bucket, ok       = ctx.Value(appctx.ParametersMergeBucketCTXKey).(string)
		)
		logger.Debug().Str("bucket", bucket).Msg("merge bucket env var")
		if ok {
			// get payout status
			logger.Debug().Str("bucket", bucket).Msg("extracting payout status")
			payoutStatus, err = custodian.ExtractPayoutStatus(ctx, s.s3Client, bucket)
			if err != nil {
				return nil, fmt.Errorf("failed to get payout status parameters: %w", err)
			}
			logger.Debug().Str("bucket", bucket).Str("payout status", fmt.Sprintf("%+v", *payoutStatus)).Msg("payout status")

			// get the custodian regions
			logger.Debug().Str("bucket", bucket).Msg("extracting custodian regions")
			custodianRegions, err = custodian.ExtractCustodianRegions(ctx, s.s3Client, bucket)
			if err != nil {
				return nil, fmt.Errorf("failed to get custodian regions parameters: %w", err)
			}
			logger.Debug().Str("bucket", bucket).Str("custodian regions", fmt.Sprintf("%+v", *custodianRegions)).Msg("custodianRegions")
		}
		s.cacheMu.Lock()
		s.lastPayoutStatus = payoutStatus         // update the payout status
		s.lastCustodianRegions = custodianRegions // update the custodian regions
		s.lastPollTime = time.Now()               // update the time to now
		s.cacheMu.Unlock()
	}
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()
	payoutStatus := s.lastPayoutStatus
	custodianRegions := s.lastCustodianRegions

	params := &ParametersV1{
		PayoutStatus:     payoutStatus,
		CustodianRegions: custodianRegions,
		BATRate:          rate,
		AutoContribute: AutoContribute{
			DefaultChoice: defaultChoice,
			Choices:       choices,
		},
		Tips: Tips{
			DefaultTipChoices:     getTipChoices(ctx),
			DefaultMonthlyChoices: getMonthlyChoices(ctx),
		},
	}

	vbatDeadline, ok := ctx.Value(appctx.ParametersVBATDeadlineCTXKey).(time.Time)
	if ok {
		params.VBATDeadline = vbatDeadline
	}

	transition, ok := ctx.Value(appctx.ParametersTransitionCTXKey).(bool)
	if ok {
		params.Transition = transition
	}

	return params, nil
}
