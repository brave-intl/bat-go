package rewards

import (
	"context"
	"errors"
	"fmt"
	"io"
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

const (
	reqBodyLimit10MB = 10 << 20
)

type s3Service interface {
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

type CardsConfig struct {
	Bucket string
	Key    string
}

type Config struct {
	TOSVersion int
	Cards      *CardsConfig
}

type Service struct {
	cfg                  *Config
	lastPollTime         time.Time
	lastPayoutStatus     *custodian.PayoutStatus
	lastCustodianRegions *custodian.Regions
	cacheMu              *sync.RWMutex
	jobs                 []srv.Job
	ratios               ratios.Client
	// Stop using the interface in lib and use s3Service.
	s3Client appaws.S3GetObjectAPI
	s3Svc    s3Service
}

func (s *Service) Jobs() []srv.Job {
	return s.jobs
}

// InitService initializes a new instance of the rewards service.
func InitService(ctx context.Context, cfg *Config) (*Service, error) {
	ratiosCl, err := ratios.NewWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize ratios client: %w", err)
	}

	lg := logging.Logger(ctx, "rewards")

	awsCfg, err := appaws.BaseAWSConfig(ctx, lg)
	if err != nil {
		return nil, fmt.Errorf("failed to create base aws config: %w", err)
	}

	s3client := s3.NewFromConfig(awsCfg)

	return &Service{
		cfg:      cfg,
		cacheMu:  new(sync.RWMutex),
		jobs:     []srv.Job{},
		ratios:   ratiosCl,
		s3Client: s3client,

		s3Svc: s3client,
	}, nil
}

// GetParameters - respond to caller with the rewards parameters
func (s *Service) GetParameters(ctx context.Context, currency *BaseCurrency) (*ParametersV1, error) {
	if currency == nil {
		currency = new(BaseCurrency)
		*currency = "usd"
	}

	var currencyStr = strings.ToLower(currency.String())

	lg := logging.Logger(ctx, "rewards.GetParameters")

	rateData, err := s.ratios.FetchRate(ctx, "bat", currencyStr)
	if err != nil {
		lg.Error().Err(err).Msg("failed to fetch rate from ratios")
		return nil, fmt.Errorf("failed to fetch rate from ratios: %w", err)
	}

	if rateData == nil {
		lg.Error().Msg("empty response from ratios")
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
		lg.Debug().Str("bucket", bucket).Msg("merge bucket env var")
		if ok {
			// get payout status
			lg.Debug().Str("bucket", bucket).Msg("extracting payout status")
			payoutStatus, err = custodian.ExtractPayoutStatus(ctx, s.s3Client, bucket)
			if err != nil {
				return nil, fmt.Errorf("failed to get payout status parameters: %w", err)
			}
			lg.Debug().Str("bucket", bucket).Str("payout status", fmt.Sprintf("%+v", *payoutStatus)).Msg("payout status")

			// get the custodian regions
			lg.Debug().Str("bucket", bucket).Msg("extracting custodian regions")
			custodianRegions, err = custodian.ExtractCustodianRegions(ctx, s.s3Client, bucket)
			if err != nil {
				return nil, fmt.Errorf("failed to get custodian regions parameters: %w", err)
			}
			lg.Debug().Str("bucket", bucket).Str("custodian regions", fmt.Sprintf("%+v", *custodianRegions)).Msg("custodianRegions")
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
		params.VBATDeadline = &vbatDeadline
	}

	transition, ok := ctx.Value(appctx.ParametersTransitionCTXKey).(bool)
	if ok {
		params.Transition = transition
	}

	return params, nil
}

type CardBytes []byte

func (s *Service) GetCardsAsBytes(ctx context.Context) (CardBytes, error) {
	out, err := s.s3Svc.GetObject(ctx, &s3.GetObjectInput{Bucket: &s.cfg.Cards.Bucket, Key: &s.cfg.Cards.Key})
	if err != nil {
		return nil, err
	}
	defer func() { _ = out.Body.Close() }()

	return io.ReadAll(io.LimitReader(out.Body, reqBodyLimit10MB))
}
