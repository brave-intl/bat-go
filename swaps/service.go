package swaps

import (
	"context"
	"fmt"
	// "strings"
	"time"

	"github.com/brave-intl/bat-go/utils/clients/coingecko"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/logging"
	logutils "github.com/brave-intl/bat-go/utils/logging"
	srv "github.com/brave-intl/bat-go/utils/service"
	"github.com/gomodule/redigo/redis"
	"github.com/shopspring/decimal"
)

// NewService - create a new ratios service structure
func NewService(ctx context.Context, coingecko coingecko.Client, redis *redis.Pool) *Service {
	return &Service{
		jobs:      []srv.Job{},
		coingecko: coingecko,
		redis:     redis,
	}
}

// Service contains datastore
type Service struct {
	jobs []srv.Job
	// coingecko client
	coingecko coingecko.Client
	redis     *redis.Pool
}

// Jobs - Implement srv.JobService interface
func (s *Service) Jobs() []srv.Job {
	return s.jobs
}

// InitService creates a service using the passed context
func InitService(ctx context.Context) (context.Context, *Service, error) {
	// get logger from context
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		ctx, logger = logging.SetupLogger(ctx)
	}

	redisAddr, err := appctx.GetStringFromContext(ctx, appctx.RatiosRedisAddrCTXKey)
	if err != nil {
		logger.Error().Err(err).Msg("failed to initialize the redis client")
		return ctx, nil, fmt.Errorf("failed to initialize redis client: %w", err)
	}

	redis := &redis.Pool{
		MaxIdle:     3,
		IdleTimeout: 240 * time.Second,
		// Dial or DialContext must be set. When both are set, DialContext takes precedence over Dial.
		Dial: func() (redis.Conn, error) {
			return redis.DialURL(redisAddr)
		},
	}

	conn := redis.Get()
	defer func() {
		err := conn.Close()
		logutils.Logger(ctx, "ratios.InitService").Error().Err(err).Msg("failed to close redis conn")
	}()
	err = conn.Err()
	if err != nil {
		logger.Error().Err(err).Msg("failed to initialize the redis client")
		return ctx, nil, fmt.Errorf("failed to initialize redis client: %w", err)
	}

	client, err := coingecko.NewWithContext(ctx, redis)
	if err != nil {
		logger.Error().Err(err).Msg("failed to initialize the coingecko client")
		return ctx, nil, fmt.Errorf("failed to initialize coingecko client: %w", err)
	}

	service := NewService(ctx, client, redis)

	// ctx, err = service.initializeCoingeckoCurrencies(ctx)
	// if err != nil {
	// 	logger.Error().Err(err).Msg("failed to initialize the coingecko coin mappings")
	// 	return ctx, nil, fmt.Errorf("failed to initialize coingecko coin mappings: %w", err)
	// }

	// service.jobs = []srv.Job{
	// 	{
	// 		Func:    service.RunNextRelativeCachePrepopulationJob,
	// 		Cadence: 5 * time.Minute,
	// 		Workers: 1,
	// 	},
	// }

	// Sigh, for compatibility with existing ratios mistakes
	decimal.MarshalJSONWithoutQuotes = true
	return ctx, service, nil
}
