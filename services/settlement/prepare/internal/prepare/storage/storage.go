package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/brave-intl/bat-go/services/settlement/internal/consumer/redis"
	"github.com/brave-intl/bat-go/services/settlement/internal/payment"
)

// keyPrefix is the prefix used for the redis sorted set that stores the prepared transactions.
const keyPrefix = "txn-store-"

type RedisClient interface {
	ZAddNX(ctx context.Context, key string, args redis.MemberArgs) (int64, error)
	ZCard(ctx context.Context, key string) (int64, error)
	ZRange(ctx context.Context, key string, start, stop int64) ([]string, error)
}

type TransactionStore struct {
	rc RedisClient
}

func NewTransactionStore(rc RedisClient) *TransactionStore {
	return &TransactionStore{rc: rc}
}

// SaveTransaction inserts a transaction into storage if it is not present. SaveTransaction is idempotent.
func (a *TransactionStore) SaveTransaction(ctx context.Context, payoutID string, attestedDetail payment.AttestedDetails) error {
	_, err := a.rc.ZAddNX(ctx, keyPrefix+payoutID, redis.MemberArgs{
		Score:  float64(time.Now().Unix()),
		Member: attestedDetail,
	})
	if err != nil {
		return fmt.Errorf("error saving transaction: %w", err)
	}
	return nil
}

func (a *TransactionStore) Count(ctx context.Context, payoutID string) (int64, error) {
	r, err := a.rc.ZCard(ctx, keyPrefix+payoutID)
	if err != nil {
		return 0, fmt.Errorf("error counting transactions: %w", err)
	}
	return r, nil
}

func (a *TransactionStore) Fetch(ctx context.Context, payoutID string, start, stop int64) (any, error) {
	attestations, err := a.rc.ZRange(ctx, keyPrefix+payoutID, start, stop)
	if err != nil {
		return nil, fmt.Errorf("error fetching transactions by range: %w", err)
	}

	var ad payment.AttestedDetails
	var result []payment.AttestedDetails
	for _, s := range attestations {
		err = json.Unmarshal([]byte(s), &ad)
		if err != nil {
			return nil, fmt.Errorf("error unmarshalling attested details: %w", err)
		}
		result = append(result, ad)
	}

	return result, nil
}
