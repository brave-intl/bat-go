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
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/services/settlement/internal/consumer"
	"github.com/brave-intl/bat-go/services/settlement/internal/consumer/redis"
	"github.com/brave-intl/bat-go/services/settlement/internal/payment"
	"github.com/brave-intl/bat-go/services/settlement/prepare/internal/prepare/report"
	"github.com/brave-intl/bat-go/services/settlement/prepare/internal/prepare/storage"
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
	// prepareConfigStream is the default stream where config pertaining to the prepare phase of the payout
	// can be retrieved.
	prepareConfigStream = "prepare-config"
	// lockTimeout is the default duration a lock will remain before expiring.
	// Set this high for uploads as we don't want the lock to timeout before the upload is complete.
	lockTimeout = time.Minute * 15
	// reportContentType is the default report content type.
	reportContentType = "application/json"
	// reportUploadPartSizeMinimum is the default upload part size. AWS specify a part size must be greater than 5MB
	// so this value cannot be set to a value which will result to a part size being less than 5MB.
	reportUploadPartSizeMinimum = 10
)

type Factory interface {
	CreateConsumer(config payment.Config) (consumer.Consumer, error)
}

type PayoutConfigClient interface {
	ReadPayoutConfig(ctx context.Context) (payment.Config, error)
	SetLastProcessedPayout(ctx context.Context, config payment.Config) error
}

type ReportUploader interface {
	Upload(ctx context.Context, config payment.Config) (*report.CompletedUpload, error)
}

type ReportNotifier interface {
	Notify(ctx context.Context, payoutID, reportURI string, versionID string) error
}

type RedisLocker interface {
	AcquireLock(ctx context.Context, key string, value uuid.UUID, expiration time.Duration) (bool, error)
	ReleaseLock(ctx context.Context, key string, value uuid.UUID) (int, error)
}

// Worker defines a prepare Worker and its dependencies.
type Worker struct {
	factory      Factory
	payoutClient PayoutConfigClient
	uploader     ReportUploader // TODO move to report service
	notifier     ReportNotifier // TODO move to report service
	locker       RedisLocker    // TODO move to report service
}

// NewWorker created a new instance of prepare.Worker.
func NewWorker(payoutClient PayoutConfigClient, factory Factory, uploader ReportUploader, notifier ReportNotifier, locker RedisLocker) *Worker {
	return &Worker{
		payoutClient: payoutClient,
		factory:      factory,
		uploader:     uploader,
		notifier:     notifier,
		locker:       locker,
	}
}

// Run starts the Prepare transaction flow. This includes reading from the payout config stream, processing the
// transactions, uploading the report and finally sending a notification when complete. The Prepare flow for a given
// payout will not get marked as complete unless all steps have been successful. Payouts are processed sequentially
// as they are added to the prepare payout config stream.
func (w *Worker) Run(ctx context.Context) {
	l := logging.Logger(ctx, "Worker.Run")

	for {
		select {
		case <-ctx.Done():
			return
		default:
			pc, err := w.payoutClient.ReadPayoutConfig(ctx)
			if err != nil {
				l.Error().Err(err).Msg("error reading config")
				continue
			}

			l.Info().Interface("prepare_config", pc).Msg("processing prepare")

			c, err := w.factory.CreateConsumer(pc)
			if err != nil {
				l.Error().Err(err).Msg("error creating consumer")
				continue
			}

			if err := consume(ctx, c); err != nil {
				l.Error().Err(err).Msg("error processing payout")
				continue
			}

			if err := w.payoutClient.SetLastProcessedPayout(ctx, pc); err != nil {
				l.Error().Err(err).Msg("error setting last payout")
				continue
			}

			workerID := uuid.NewV4()
			lock, err := w.locker.AcquireLock(ctx, pc.PayoutID, workerID, lockTimeout)
			if err != nil {
				l.Error().Err(err).Msg("error setting lock")
				continue
			}

			// If another consumer has the lock it should be doing the upload, so we can stop here.
			if !lock {
				l.Warn().Interface("prepare_config", pc).Msg("could not acquire lock for upload")
				continue
			}

			completedUpload, err := w.uploader.Upload(ctx, pc)
			if err != nil {
				l.Error().Err(err).Msg("error uploading settlement report")
				continue
			}

			l.Info().Msg("sending prepared notification")

			if isNotificationEnabled() {
				if err := w.notifier.Notify(ctx, pc.PayoutID, completedUpload.Location, completedUpload.VersionID); err != nil {
					l.Error().Err(err).Msg("error sending notification")
					continue
				}
			}

			if _, err := w.locker.ReleaseLock(ctx, pc.PayoutID, workerID); err != nil {
				switch {
				case errors.Is(err, redis.ErrLockValueDoesNotMatch):
					l.Warn().Err(err).Msg("another worker has taken the lock")
				default:
					l.Error().Err(err).Msg("error removing lock")
				}
				continue
			}

			l.Info().Interface("uploaded_report", completedUpload).Msg("prepare complete")
		}
	}
}

func consume(ctx context.Context, consumer consumer.Consumer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		cancel() // calling cancel stops the consumer
	}()

	if err := consumer.Consume(ctx); err != nil {
		return fmt.Errorf("error start prepare consumer: %w", err)
	}

	return nil
}

// WorkerConfig represents the configuration for a Worker.
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
func NewWorkerConfig(opts ...Option) (*WorkerConfig, error) {
	c := &WorkerConfig{
		configStream:         prepareConfigStream,
		reportContentType:    reportContentType,
		reportUploadPartSize: reportUploadPartSizeMinimum,
	}
	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, err
		}
	}
	return c, nil
}

type Option func(c *WorkerConfig) error

// WithRedisAddress sets the redis address.
func WithRedisAddress(addr string) Option {
	return func(c *WorkerConfig) error {
		if addr == "" {
			return errors.New("redis address cannot be empty")
		}
		c.redisAddress = addr
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

// WithConfigStream sets the name of the config stream for the Worker. Defaults prepare-config stream.
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
	addr := config.redisAddress + ":6379" //TODO add port addr to ops
	rc := redis.NewClient(addr, config.redisUsername, config.redisPassword)

	pc, err := payment.NewClient(config.paymentURL)
	if err != nil {
		return nil, fmt.Errorf("error creating payment client: %w", err)
	}

	csc := payment.NewConfigClient(rc, config.configStream)

	fy := NewFactory(rc, pc)

	txnStore := storage.NewTransactionStore(rc)

	l := logging.Logger(ctx, "prepare.aws")
	awsConf, err := awsutils.BaseAWSConfig(ctx, l)
	if err != nil {
		return nil, fmt.Errorf("error creating aws base config: %w", err)
	}
	s3Client := awsutils.NewClient(awsConf)

	mpu := report.NewMultiPartUploader(txnStore, s3Client, awsutils.S3UploadConfig{
		Bucket:      config.reportBucket,
		ContentType: config.reportContentType,
		PartSize:    config.reportUploadPartSize,
	})

	p := snslibs.New(awsConf)
	n := report.NewNotifier(p, config.notificationTopic, backoff.Retry)

	w := NewWorker(csc, fy, mpu, n, rc)

	return w, nil
}
