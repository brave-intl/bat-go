package payout

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/brave-intl/bat-go/libs/clients/payment"
	"github.com/brave-intl/bat-go/services/settlement/event"
	"github.com/go-redis/redis/v8"
)

const (
	// defaultStreamValue where no last processed message key exists for a given config stream we set the value
	// to a default id of `0` i.e. the first message in the stream.
	defaultStreamValue = "0"
	// lastProcessedMessageKeySuffix is the suffix used to create the last processed message id.
	// This should be combined with name of the config stream.
	lastProcessedMessageKeySuffix = "-last-processed-message-id"
	// preparedTransactionsPrefix is the prefix used for the redis sorted set that stores the prepared transactions.
	preparedTransactionsPrefix = "prepared-transactions-"
)

var (
	errRedisMessageKeyNotFound = errors.New("redis message key not found")
)

type (
	// Config defines the configuration values for a payout stream.
	Config struct {
		PayoutID      string `json:"payoutId"`
		Stream        string `json:"stream"`
		ConsumerGroup string `json:"consumerGroup"`
		Count         int    `json:"count"`
		xRedisID      string
	}
)

// RedisConfigStreamClient implements the API to interact with the Redis payout configuration stream.
type RedisConfigStreamClient struct {
	rc                      *event.RedisClient
	configStream            string
	lastProcessedMessageKey string
}

// NewRedisConfigStreamClient creates a new instance of NewRedisConfigStreamClient.
func NewRedisConfigStreamClient(redisClient *event.RedisClient, configStream string) *RedisConfigStreamClient {
	l := fmt.Sprintf("%s%s", configStream, lastProcessedMessageKeySuffix)
	return &RedisConfigStreamClient{
		rc:                      redisClient,
		configStream:            configStream,
		lastProcessedMessageKey: l,
	}
}

// ReadPayoutConfig reads the most recent payout config from the provided config stream. This func will block
// indefinitely waiting for new messages.
func (r *RedisConfigStreamClient) ReadPayoutConfig(ctx context.Context) (*Config, error) {
	// Get the last processed redis message messageID from the key.
	// If no key exists then set the key to the default.
	messageID, err := r.rc.Get(ctx, r.lastProcessedMessageKey).Result()
	if err != nil {
		switch {
		// If the key does not exist set the value to default using `SETNX` to avoid a race condition.
		// This should only happen on the first attempt to retrieve the key or if the key has been deleted.
		case errors.Is(err, redis.Nil):
			if _, err := r.rc.SetNX(ctx, r.lastProcessedMessageKey, defaultStreamValue, 0).Result(); err != nil {
				return nil, fmt.Errorf("read payout config: error calling setnx: %w", err)
			}
			messageID = defaultStreamValue
		default:
			return nil, fmt.Errorf("error getting last processed message: %w", err)
		}
	}

	messages, err := r.rc.Read(ctx, []string{r.configStream, messageID}, 1, 0)
	if err != nil {
		return nil, fmt.Errorf("error reading payout config message: %w", err)
	}

	if len(messages) != 1 {
		return nil, nil
	}

	var config Config
	err = json.Unmarshal([]byte(messages[0].Body), &config)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling payout config message: %w", err)
	}

	XRedisID, ok := messages[0].Headers[event.XRedisIDKey]
	if !ok {
		return nil, fmt.Errorf("error redis message header not found: %w", errRedisMessageKeyNotFound)
	}
	config.xRedisID = XRedisID

	return &config, nil
}

// SetLastPayout sets the last processed message for the RedisConfigStreamClient stream set at initialization.
func (r *RedisConfigStreamClient) SetLastPayout(ctx context.Context, config Config) error {
	_, err := r.rc.Set(ctx, r.lastProcessedMessageKey, config.xRedisID, 0).Result()
	if err != nil {
		return fmt.Errorf("error setting last processed id: %w", err)
	}
	return nil
}

func (r *RedisConfigStreamClient) AddPreparedTransaction(ctx context.Context, payoutID string, attestedTransaction payment.AttestedTransaction) error {
	_, err := r.rc.ZAddNX(ctx, preparedTransactionsPrefix+payoutID, &redis.Z{
		Score:  float64(time.Now().Unix()),
		Member: attestedTransaction,
	}).Result()
	if err != nil {
		return fmt.Errorf("error adding prepared transaction: %w", err)
	}
	return nil
}

// GetNumberOfPreparedTransactions returns the number of prepared transactions for the given payout.
func (r *RedisConfigStreamClient) GetNumberOfPreparedTransactions(ctx context.Context, payoutID string) (int64, error) {
	c, err := r.rc.ZCard(ctx, preparedTransactionsPrefix+payoutID).Result()
	if err != nil {
		return 0, fmt.Errorf("error getting number of prepared transactions: %w", err)
	}
	return c, nil
}

// GetPreparedTransactionsByRange returns the prepared transactions for a given payout between a specified range.
func (r *RedisConfigStreamClient) GetPreparedTransactionsByRange(ctx context.Context, payoutID string, start, stop int64) ([]payment.AttestedTransaction, error) {
	m, err := r.rc.ZRange(ctx, preparedTransactionsPrefix+payoutID, start, stop).Result()
	if err != nil {
		return nil, fmt.Errorf("error getting prepared transactions by range: %w", err)
	}

	var txn payment.AttestedTransaction
	var txns []payment.AttestedTransaction
	for _, s := range m {
		err = json.Unmarshal([]byte(s), &txn)
		if err != nil {
			return nil, fmt.Errorf("error unmarshalling prepared transactions: %w", err)
		}
		txns = append(txns, txn)
	}

	return txns, nil
}
