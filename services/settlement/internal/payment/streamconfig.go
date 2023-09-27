package payment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/brave-intl/bat-go/services/settlement/internal/consumer"
	"github.com/brave-intl/bat-go/services/settlement/internal/consumer/redis"
)

const (
	// defaultStreamValue where no last processed message key exists for a given config stream we set the value
	// to a default id of `0` i.e. the first message in the stream.
	defaultStreamValue = "0"
	// lastProcessedMessageKeySuffix is the suffix used to create the last processed message id.
	// This should be combined with name of the config stream.
	lastProcessedMessageKeySuffix = "-last-processed-message-id"
	// dataKey is the key used to retrieve the stream.Message value from the redis stream message.
	dataKey = "data"
)

var errDataKeyNotFound = errors.New("data key not found")

type RedisClient interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, args redis.SetArgs) (string, error)
	SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error)
	XRead(ctx context.Context, args redis.XReadArgs) ([]redis.XMessage, error)
}

// ConfigClient implements the API to interact with the Redis payout configuration stream.
type ConfigClient struct {
	rc     RedisClient
	stream string
}

// NewConfigClient creates a new instance of NewRedisConfigStreamClient.
func NewConfigClient(redis RedisClient, stream string) *ConfigClient {
	return &ConfigClient{
		rc:     redis,
		stream: stream,
	}
}

// Config defines the configuration values for a payout stream.
type Config struct {
	PayoutID      string `json:"payoutId"`
	Stream        string `json:"stream"`
	ConsumerGroup string `json:"consumerGroup"`
	Count         int    `json:"count"`
	xRedisID      string
}

// ReadPayoutConfig reads the most recent payout config from the provided config stream. This func will block
// indefinitely waiting for new messages.
func (r *ConfigClient) ReadPayoutConfig(ctx context.Context) (Config, error) {
	key := r.stream + lastProcessedMessageKeySuffix
	// Get the last processed redis message messageID from the key.
	// If no key exists then set the key to the default.
	messageID, err := r.rc.Get(ctx, key)
	if err != nil {
		switch {
		// If the key does not exist set the value to default using set if not exist `SETNX` to avoid a race condition.
		// This should only happen on the first attempt to retrieve the key or if the key has been deleted.
		case errors.Is(err, redis.ErrKeyDoesNotExist):
			if _, err := r.rc.SetNX(ctx, key, defaultStreamValue, 0); err != nil {
				return Config{}, fmt.Errorf("read payout config: error calling setnx: %w", err)
			}
			messageID = defaultStreamValue
		default:
			return Config{}, fmt.Errorf("error getting last processed message: %w", err)
		}
	}

	xMsg, err := r.rc.XRead(ctx, redis.XReadArgs{
		Stream:    r.stream,
		MessageID: messageID,
		Count:     1,
		Block:     0,
	})
	if err != nil {
		return Config{}, fmt.Errorf("error reading payout config message: %w", err)
	}

	if len(xMsg) != 1 {
		return Config{}, nil
	}

	d, ok := xMsg[0].Values[dataKey]
	if !ok {
		return Config{}, errDataKeyNotFound
	}

	s, ok := d.(string)
	if !ok {
		return Config{}, errors.New("error invalid data type")
	}

	message, err := consumer.NewMessageFromString(s)
	if err != nil {
		return Config{}, fmt.Errorf("error creating new message: %w", err)
	}

	var config Config
	err = json.Unmarshal([]byte(message.Body), &config)
	if err != nil {
		return Config{}, fmt.Errorf("error unmarshalling payout config message: %w", err)
	}

	config.xRedisID = xMsg[0].ID

	return config, nil
}

// SetLastProcessedPayout sets the last processed message for the ConfigClient stream set at initialization.
func (r *ConfigClient) SetLastProcessedPayout(ctx context.Context, payoutConfig Config) error {
	_, err := r.rc.Set(ctx, redis.SetArgs{
		Key:        r.stream + lastProcessedMessageKeySuffix,
		Value:      payoutConfig.xRedisID,
		Expiration: 0, // explicitly set no expiration time
	})
	if err != nil {
		return fmt.Errorf("error setting last processed id: %w", err)
	}
	return nil
}
