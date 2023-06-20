package internal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	awsutils "github.com/brave-intl/bat-go/libs/aws"
	snslibs "github.com/brave-intl/bat-go/libs/aws/sns"
	"github.com/brave-intl/bat-go/libs/backoff"
	"github.com/brave-intl/bat-go/libs/clients/payment"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/services/settlement/event"
	"github.com/brave-intl/bat-go/services/settlement/payout"
	"github.com/brave-intl/bat-go/services/settlement/prepare/internal/factory"
	"github.com/brave-intl/bat-go/services/settlement/prepare/internal/report"

	uuid "github.com/satori/go.uuid"
)

func isNotificationEnabled() bool {
	var toggle = false
	if os.Getenv("NOTIFICATION_ENABLED") != "" {
		var err error
		toggle, err = strconv.ParseBool(os.Getenv("NOTIFICATION_ENABLED"))
		if err != nil {
			return false
		}
	}
	return toggle
}

const (
	// prepareConfigStream is the name of the stream where config pertaining to the prepare phase of the payout
	// can be retrieved.
	prepareConfigStream = "prepare-config"

	// lockTimeout defines the duration a lock will remain before expiring.
	// Set this high for uploads as we don't want the lock to timeout before the upload is complete.
	lockTimeout = time.Minute * 15
)

type ConsumerFactory interface {
	CreateConsumer(config payout.Config) (event.Consumer, error)
}

type ConfigStreamAPI interface {
	ReadPayoutConfig(ctx context.Context) (*payout.Config, error)
	SetLastPayout(ctx context.Context, config payout.Config) error
}

type PreparedTransactionUploader interface {
	Upload(ctx context.Context, config payout.Config) (*report.CompletedUpload, error)
}

type NotificationAPI interface {
	SendNotification(ctx context.Context, payoutID, reportURI string, versionID string) error
}

type PrepareWorker struct {
	redis                       *event.RedisClient
	paymentClient               payment.Client
	consumerFactory             ConsumerFactory
	configClient                ConfigStreamAPI
	preparedTransactionUploader PreparedTransactionUploader
	notification                NotificationAPI
}

func NewPrepareWorker(ctx context.Context) (*PrepareWorker, error) {
	redisAddress, ok := ctx.Value(appctx.SettlementRedisAddressCTXKey).(string)
	if !ok {
		return nil, fmt.Errorf("new prepare worker: error retrieving redis address")
	}

	redisUsername, ok := ctx.Value(appctx.SettlementRedisUsernameCTXKey).(string)
	if !ok {
		return nil, fmt.Errorf("new prepare worker: error retrieving redis username")
	}

	redisPassword, ok := ctx.Value(appctx.SettlementRedisPasswordCTXKey).(string)
	if !ok {
		return nil, fmt.Errorf("new prepare worker: error retrieving redis password")
	}

	redisAddresses := []string{fmt.Sprintf("%s:6379", redisAddress)}
	redis, err := event.NewRedisClient(redisAddresses, redisUsername, redisPassword)
	if err != nil {
		return nil, fmt.Errorf("new prepare worker: error creating redis client: %w", err)
	}

	paymentURL, ok := ctx.Value(appctx.PaymentServiceURLCTXKey).(string)
	if !ok {
		return nil, fmt.Errorf("new prepare worker: error retrieving payment url")
	}

	logger := logging.Logger(ctx, "PrepareWorker")

	c := payout.NewRedisConfigStreamClient(redis, prepareConfigStream)

	cfg, err := awsutils.BaseAWSConfig(ctx, logger)
	if err != nil {
		return nil, fmt.Errorf("new notify worker: error creating S3 client config: %w", err)
	}

	s3Client, err := awsutils.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("new notify worker: error creating S3 client: %w", err)
	}

	bucket, ok := ctx.Value(appctx.SettlementPayoutReportBucketCTXKey).(string)
	if !ok {
		return nil, fmt.Errorf("new prepare worker: error retrieving bucket")
	}

	//TODO remove this to default
	contentType, ok := ctx.Value(appctx.SettlementPayoutReportContentTypeCTXKey).(string)
	if !ok {
		contentType = "application/json"
		//return nil, fmt.Errorf("new prepare worker: error retrieving content type")
	}

	partSize, ok := ctx.Value(appctx.SettlementPayoutReportUploadPartSizeCTXKey).(int64)
	if !ok {
		partSize = 10
		//		return nil, fmt.Errorf("new prepare worker: error retrieving upload part size")
	}

	s3UploadConfig := awsutils.S3UploadConfig{
		Bucket:      bucket,
		ContentType: contentType,
		PartSize:    partSize,
	}

	uploader := report.NewPreparedTransactionUploadClient(c, s3Client, s3UploadConfig)

	publisher := snslibs.New(cfg)

	topic, ok := ctx.Value(appctx.SettlementSNSNotificationTopicARNCTXKey).(string)
	if !ok {
		topic = "TODO"
		// return nil, fmt.Errorf("error getting sns topic")
	}

	n := report.NewNotificationClient(publisher, topic, backoff.Retry)

	p := payment.New(paymentURL)

	f := factory.NewPrepareConsumer(redis, c, p)

	return &PrepareWorker{
		redis:                       redis,
		paymentClient:               p,
		consumerFactory:             f,
		configClient:                c,
		preparedTransactionUploader: uploader,
		notification:                n,
	}, nil
}

func (p *PrepareWorker) Run(ctx context.Context) {
	logger := logging.Logger(ctx, "PrepareWorker.Run")

	for {
		select {
		case <-ctx.Done():
			return
		default:

			pc, err := p.configClient.ReadPayoutConfig(ctx)
			if err != nil {
				logger.Error().Err(err).Msg("error reading config")
				continue
			}

			if pc == nil {
				continue
			}
			config := *pc

			logger.Info().Interface("prepare_config", config).
				Msg("processing prepare")

			c, err := p.consumerFactory.CreateConsumer(config)
			if err != nil {
				logger.Error().Err(err).Msg("error creating consumer")
				continue
			}

			err = Runner(ctx, c)
			if err != nil {
				logger.Error().Err(err).Msg("error processing payout")
				continue
			}

			err = p.configClient.SetLastPayout(ctx, config)
			if err != nil {
				logger.Error().Err(err).Msg("error setting last payout")
				continue
			}

			// Acquire a lock for this worker.
			workerID := uuid.NewV4()
			lock, err := p.redis.AcquireLock(ctx, config.PayoutID, workerID, lockTimeout)
			if err != nil {
				logger.Error().Err(err).Msg("error setting lock")
				continue
			}

			// If another consumer has the lock it should be doing the upload.
			if !lock {
				logger.Warn().Interface("prepare_config", config).
					Msg("could not acquire lock for upload")
				continue
			}

			completedUpload, err := p.preparedTransactionUploader.Upload(ctx, config)
			if err != nil {
				logger.Error().Err(err).Msg("error uploading settlement report")
				continue
			}

			logger.Info().Msg("sending prepared notification")

			if isNotificationEnabled() {
				err = p.notification.SendNotification(ctx, config.PayoutID, completedUpload.Location, completedUpload.VersionID)
				if err != nil {
					logger.Error().Err(err).Msg("error sending notification")
				}
			}

			_, err = p.redis.ReleaseLock(ctx, config.PayoutID, workerID)
			if err != nil {
				switch {
				// Check if another worker has taken lock since we took it.
				case errors.Is(err, event.ErrLockValueDoesNotMatch):
					logger.Warn().Msg("warning another worker has taken the lock")
				default:
					logger.Error().Err(err).Msg("error removing lock")
				}
				continue
			}

			logger.Info().Interface("uploaded_report", completedUpload).
				Msg("prepare complete")
		}
	}
}

// TODO select?
func Runner(ctx context.Context, consumer event.Consumer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		cancel() // calling cancel stops the consumer
	}()

	resultC := make(chan error)
	err := consumer.Start(ctx, resultC)
	if err != nil {
		return fmt.Errorf("error start prepare consumer: %w", err)
	}

	return <-resultC
}

//func NewWorker(options ...Option) (*PrepareWorker, error) {
//	w := new(PrepareWorker)
//	for _, option := range options {
//		if err := option(w); err != nil {
//			return nil, err
//		}
//	}
//	return w, nil
//}
//
//func T() {
//	w, _ := NewWorker(
//		WithRedisClient(),
//		WithPaymentClient(),
//		WithRedisUploader(),
//		WithNotificationPublisher()
//		)
//}
//
//type Option func(worker *PrepareWorker) error
//
//func WithRedisClient(address, username, password string) Option {
//	return func(w *PrepareWorker) error {
//		a := []string{fmt.Sprintf("%s:6379", address)}
//		r, err := event.NewRedisClient(a, username, password)
//		if err != nil {
//			return fmt.Errorf("new prepare worker: error creating redis client: %w", err)
//		}
//		w.redis = r
//		return nil
//	}
//}
//
//func WithPaymentClient(url string) Option {
//	return func(w *PrepareWorker) error {
//		w.paymentClient = payment.New(url)
//		return nil
//	}
//}
//
//func WithRedisUploader(url string) Option {
//	return func(w *PrepareWorker) error {
//		w.paymentClient = payment.New(url)
//		return nil
//	}
//}
//
//func WithNotificationPublisher() Option {
//	return func(w *PrepareWorker) error {
//		w.paymentClient = payment.New(url)
//		return nil
//	}
//}
