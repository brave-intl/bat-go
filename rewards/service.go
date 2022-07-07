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
	"github.com/aws/aws-sdk-go-v2/service/s3"
	awslogging "github.com/aws/smithy-go/logging"
	"github.com/brave-intl/bat-go/utils/clients/ratios"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/inputs"
	"github.com/brave-intl/bat-go/utils/logging"
	srv "github.com/brave-intl/bat-go/utils/service"
	"github.com/rs/zerolog"
)

// PayoutStatus - current state of the payout status
type PayoutStatus struct {
	Unverified string `json:"unverified" valid:"in(off|processing|complete)"`
	Uphold     string `json:"uphold" valid:"in(off|processing|complete)"`
	Gemini     string `json:"gemini" valid:"in(off|processing|complete)"`
	Bitflyer   string `json:"bitflyer" valid:"in(off|processing|complete)"`
}

// S3GetObjectAPI - interface to allow for a GetObject mock
type S3GetObjectAPI interface {
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

func extractPayoutStatus(ctx context.Context, client S3GetObjectAPI, bucket, object string) (*PayoutStatus, error) {
	logger := logging.Logger(ctx, "rewards.extractPayoutStatus")
	// get the object with the client
	out, err := client.GetObject(
		ctx, &s3.GetObjectInput{
			Bucket: &bucket,
			Key:    &object,
		})
	if err != nil {
		return nil, fmt.Errorf("failed to get payout status: %w", err)
	}
	defer func() {
		err := out.Body.Close()
		logger.Error().Err(err).Msg("failed to close s3 result body")
	}()
	var payoutStatus = new(PayoutStatus)

	// parse body json
	if err := inputs.DecodeAndValidateReader(ctx, payoutStatus, out.Body); err != nil {
		return nil, payoutStatus.HandleErrors(err)
	}

	return payoutStatus, nil
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

type appLogger struct {
	*zerolog.Logger
}

// Logf - implement smithy-go/logging.Logger
func (al *appLogger) Logf(classification awslogging.Classification, format string, v ...interface{}) {
	al.Debug().Msg(fmt.Sprintf(format, v...))
}

// NewService - create a new rewards service structure
func NewService(ctx context.Context, ratio ratios.Client, s3client S3GetObjectAPI) (*Service, error) {
	logger := logging.Logger(ctx, "rewards.NewService")

	logger.Info().Msg("getting aws region")
	// get the aws region from ctx
	region, ok := ctx.Value(appctx.AWSRegionCTXKey).(string)
	if !ok || len(region) == 0 {
		region = "us-west-2"
	}

	logger.Info().Msg("checking s3 client")
	if s3client == nil {
		logger.Info().Str("region", region).Msg("setting up s3 client")
		// aws config
		cfg, err := config.LoadDefaultConfig(
			ctx,
			config.WithLogger(&appLogger{logger}),
			config.WithRegion(region))
		if err != nil {
			return nil, fmt.Errorf("failed to load aws config: %w", err)
		}
		s3client = s3.NewFromConfig(cfg)
	}

	logger.Info().Str("s3client", fmt.Sprintf("%+v", s3client)).Msg("setup s3 client")
	return &Service{
		jobs:     []srv.Job{},
		ratios:   ratio,
		s3Client: s3client,
	}, nil
}

// Service contains datastore
type Service struct {
	jobs []srv.Job
	// ratios client
	ratios            ratios.Client
	payoutStatusMutex *sync.RWMutex
	payoutStatus      *PayoutStatus
	s3Client          S3GetObjectAPI
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

	// merge in static s3 attributes into response
	var (
		payoutStatus *PayoutStatus
		bucket, ok   = ctx.Value(appctx.ParametersMergeBucketCTXKey).(string)
	)
	logger.Debug().Str("bucket", bucket).Msg("merge bucket env var")
	if ok {
		logger.Debug().Str("bucket", bucket).Msg("extracting payout status")
		payoutStatus, err = extractPayoutStatus(ctx, s.s3Client, bucket, "payout-status.json")
		if err != nil {
			return nil, fmt.Errorf("failed to get payout status parameters: %w", err)
		}
		logger.Debug().Str("bucket", bucket).Str("payout status", fmt.Sprintf("%+v", *payoutStatus)).Msg("payout status")
	}

	return &ParametersV1{
		PayoutStatus: payoutStatus,
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
