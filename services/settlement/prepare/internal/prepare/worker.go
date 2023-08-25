package prepare

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
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/services/settlement/event"
	"github.com/brave-intl/bat-go/services/settlement/payout"
	"github.com/brave-intl/bat-go/services/settlement/prepare/internal/prepare/factory"
	"github.com/brave-intl/bat-go/services/settlement/prepare/internal/prepare/report"
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
	// reportContentType is the default report content type.
	reportContentType = "application/json"
	// reportUploadPartSizeMinimum is the default upload part size. AWS specify a part size must be greater than 5MB
	// so this value cannot be set to a value which will result to a part size being less than 5MB.
	reportUploadPartSizeMinimum = 10
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

// Worker defines a prepare worker and its dependencies.
type Worker struct {
	redis                       *event.RedisClient
	paymentClient               payment.Client
	consumerFactory             ConsumerFactory
	configStream                ConfigStreamAPI
	preparedTransactionUploader PreparedTransactionUploader
	notifier                    NotificationAPI
}

// NewWorker created a new instance of prepare.Worker.
func NewWorker(redisClient *event.RedisClient, paymentClient payment.Client, configStreamClient ConfigStreamAPI,
	consumerFactory ConsumerFactory, preparedTransactionUploadClient PreparedTransactionUploader,
	notificationClient NotificationAPI) *Worker {
	return &Worker{
		redis:                       redisClient,
		paymentClient:               paymentClient,
		configStream:                configStreamClient,
		consumerFactory:             consumerFactory,
		preparedTransactionUploader: preparedTransactionUploadClient,
		notifier:                    notificationClient,
	}
}

// Run starts the Prepare transaction flow. This includes reading from the payout config stream, processing the
// transactions, uploading the report and finally sending a notification when complete. The Prepare flow for a given
// payout will not get marked as complete unless all steps have been successful. Payouts are processed sequentially
// as they are added to the prepare payout config stream.
func (p *Worker) Run(ctx context.Context) {
	logger := logging.Logger(ctx, "Worker.Run")

	for {
		select {
		case <-ctx.Done():
			return
		default:
			pc, err := p.configStream.ReadPayoutConfig(ctx)
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

			err = consume(ctx, c)
			if err != nil {
				logger.Error().Err(err).Msg("error processing payout")
				continue
			}

			err = p.configStream.SetLastPayout(ctx, config)
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
				err = p.notifier.SendNotification(ctx, config.PayoutID, completedUpload.Location, completedUpload.VersionID)
				if err != nil {
					logger.Error().Err(err).Msg("error sending notification")
				}
			}

			_, err = p.redis.ReleaseLock(ctx, config.PayoutID, workerID)
			if err != nil {
				switch {
				// Check if another worker has taken lock since we took it.
				case errors.Is(err, event.ErrLockValueDoesNotMatch):
					logger.Warn().Msg("another worker has taken the lock")
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

func consume(ctx context.Context, consumer event.Consumer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		cancel() // calling cancel stops the consumer
	}()

	resultC := make(chan error)
	err := consumer.Start(ctx, resultC)
	if err != nil {
		return fmt.Errorf("error start prepare consumer: %w", err)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err = <-resultC:
		return err
	}
}

// WorkerConfig hold the configuration for a Worker.
type WorkerConfig struct {
	redisAddress         string
	redisUsername        string
	redisPassword        string
	paymentURL           string
	configStream         string
	reportBucket         string
	reportContentType    string
	reportUploadPartSize int64
	notificationTopic    string
}

// NewWorkerConfig creates and instance of WorkerConfig with the given options.
func NewWorkerConfig(options ...Option) (*WorkerConfig, error) {
	c := &WorkerConfig{
		configStream:         prepareConfigStream,
		reportContentType:    reportContentType,
		reportUploadPartSize: reportUploadPartSizeMinimum,
	}
	for _, option := range options {
		if err := option(c); err != nil {
			return nil, err
		}
	}
	return c, nil
}

type Option func(worker *WorkerConfig) error

// WithRedisAddress sets the redis address.
func WithRedisAddress(address string) Option {
	return func(c *WorkerConfig) error {
		if address == "" {
			return errors.New("redis address cannot be empty")
		}
		c.redisAddress = address
		return nil
	}
}

// WithRedisUsername sets the redis username.
func WithRedisUsername(username string) Option {
	return func(c *WorkerConfig) error {
		if username == "" {
			return errors.New("redis username cannot be empty")
		}
		c.redisUsername = username
		return nil
	}
}

// WithRedisPassword set the redis password.
func WithRedisPassword(password string) Option {
	return func(c *WorkerConfig) error {
		if password == "" {
			return errors.New("redis password cannot be empty")
		}
		c.redisPassword = password
		return nil
	}
}

// WithPaymentClient sets the url for the payment service.
func WithPaymentClient(url string) Option {
	return func(c *WorkerConfig) error {
		if url == "" {
			return errors.New("payment url cannot be empty")
		}
		c.paymentURL = url
		return nil
	}
}

// WithConfigStream sets the name of the config stream for the worker. Defaults prepare-config stream.
func WithConfigStream(stream string) Option {
	return func(c *WorkerConfig) error {
		if stream == "" {
			return errors.New("config stream cannot be empty")
		}
		c.configStream = stream
		return nil
	}
}

// WithReportBucket set the name of the bucket for uploading the prepared transaction report.
func WithReportBucket(reportBucket string) Option {
	return func(c *WorkerConfig) error {
		if reportBucket == "" {
			return errors.New("report bucket cannot be empty")
		}
		c.reportBucket = reportBucket
		return nil
	}
}

// WithReportContentType the content type of the report. Defaults to application/json.
func WithReportContentType(reportContentType string) Option {
	return func(c *WorkerConfig) error {
		if reportContentType == "" {
			return errors.New("report content type cannot be empty")
		}
		c.reportContentType = reportContentType
		return nil
	}
}

// WithReportUploadPartSize sets the size of the report upload parts. Defaults to the minimum size.
func WithReportUploadPartSize(reportUploadPartSize int64) Option {
	return func(c *WorkerConfig) error {
		if reportUploadPartSize < reportUploadPartSizeMinimum {
			return fmt.Errorf("report upload part size cannont be less than %d", reportUploadPartSizeMinimum)
		}
		c.reportUploadPartSize = reportUploadPartSize
		return nil
	}
}

// WithNotificationTopic sets the SNS notification topic. This is used to send the prepared report
// details to so that signers are notified.
func WithNotificationTopic(notificationTopic string) Option {
	return func(c *WorkerConfig) error {
		if notificationTopic == "" {
			return errors.New("notification topic cannot be empty")
		}
		c.notificationTopic = notificationTopic
		return nil
	}
}

// CreateWorker in a factory method to create a new instance of prepare.Worker.
func CreateWorker(ctx context.Context, config *WorkerConfig) (*Worker, error) {
	redisAddress := config.redisAddress + ":6379" //TODO add port address to ops
	redisClient := event.NewRedisClient(redisAddress, config.redisUsername, config.redisPassword)

	paymentClient := payment.New(config.paymentURL)

	configStreamClient := payout.NewRedisConfigStreamClient(redisClient, config.configStream)

	consumerFactory := factory.NewPrepareConsumer(redisClient, configStreamClient, paymentClient)

	logger := logging.Logger(ctx, "Worker")

	baseAWSConfig, err := awsutils.BaseAWSConfig(ctx, logger)
	if err != nil {
		return nil, fmt.Errorf("create prepare worker: error creating aws base config: %w", err)
	}

	s3Client := awsutils.NewClient(baseAWSConfig)

	s3UploadConfig := awsutils.S3UploadConfig{
		Bucket:      config.reportBucket,
		ContentType: config.reportContentType,
		PartSize:    config.reportUploadPartSize,
	}
	preparedTransactionUploadClient := report.NewPreparedTransactionUploadClient(configStreamClient, s3Client, s3UploadConfig)

	publisher := snslibs.New(baseAWSConfig)
	notificationClient := report.NewNotificationClient(publisher, config.notificationTopic, backoff.Retry)

	worker := NewWorker(redisClient, paymentClient, configStreamClient,
		consumerFactory, preparedTransactionUploadClient, notificationClient)

	return worker, nil
}
