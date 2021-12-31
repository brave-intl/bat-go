package ratios

import (
	"context"
	"fmt"
	"time"

	"github.com/brave-intl/bat-go/utils/clients/coingecko"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/logging"
	srv "github.com/brave-intl/bat-go/utils/service"
	"github.com/shopspring/decimal"
)

// NewService - create a new ratios service structure
func NewService(ctx context.Context, coingecko coingecko.Client) *Service {
	return &Service{
		jobs:      []srv.Job{},
		coingecko: coingecko,
	}
}

// Service contains datastore
type Service struct {
	jobs []srv.Job
	// coingecko client
	coingecko coingecko.Client
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

	client, err := coingecko.NewWithContext(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("failed to initialize the coingecko client")
		return ctx, nil, fmt.Errorf("failed to initialize coingecko client: %w", err)
	}
	service := NewService(ctx, client)

	ctx, err = service.initializeCoingeckoCurrencies(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("failed to initialize the coingecko coin mappings")
		return ctx, nil, fmt.Errorf("failed to initialize coingecko coin mappings: %w", err)
	}

	// Sigh, for compatibility with existing ratios mistakes
	decimal.MarshalJSONWithoutQuotes = true

	return ctx, service, nil
}

type RelativeResponse struct {
	Payload     coingecko.SimplePriceResponse `json:"payload"`
	LastUpdated time.Time                     `json:"lastUpdated"`
}

// GetRelative - respond to caller with the relative exchange rates
func (s *Service) GetRelative(ctx context.Context, coinIds CoingeckoCoinList, vsCurrencies CoingeckoVsCurrencyList, duration CoingeckoDuration) (*RelativeResponse, error) {
	// get logger from context
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		ctx, logger = logging.SetupLogger(ctx)
	}

	rates, err := s.coingecko.FetchSimplePrice(ctx, coinIds.String(), vsCurrencies.String(), duration == "1d")
	if err != nil {
		logger.Error().Err(err).Msg("failed to fetch price from coingecko")
		return nil, fmt.Errorf("failed to fetch price from coingecko: %w", err)
	}

	if duration != "1d" {
		if len(coinIds) == 1 {
			// request history for duration to calculate change
			chart, err := s.coingecko.FetchMarketChart(ctx, coinIds[0].String(), vsCurrencies[0].String(), duration.ToDays())
			if err != nil {
				logger.Error().Err(err).Msg("failed to fetch chart from coingecko")
				return nil, fmt.Errorf("failed to fetch chart from coingecko: %w", err)
			}

			tmp := map[string]map[string]decimal.Decimal(*rates)
			current := tmp[coinIds[0].String()][vsCurrencies[0].String()]
			previous := chart.Prices[0][1]
			change := current.Sub(previous).Div(previous).Mul(decimal.NewFromFloat(100))

			tmp[coinIds[0].String()][vsCurrencies[0].String()+"_timeframe_change"] = change
			r := coingecko.SimplePriceResponse(tmp)
			rates = &r
		} else {
			// fill change with 0s ( it's unused )
			out := map[string]map[string]decimal.Decimal{}
			for k, v := range *rates {
				innerOut := map[string]decimal.Decimal{}
				for kk, vv := range v {
					innerOut[kk+"_timeframe_change"] = decimal.Zero
					innerOut[kk] = vv
				}
				out[k] = innerOut
			}
			tmp := coingecko.SimplePriceResponse(out)
			rates = &tmp
		}
	}

	return &RelativeResponse{
		Payload:     mapSimplePriceResponse(ctx, *rates),
		LastUpdated: time.Now(),
	}, nil
}

type HistoryResponse struct {
	Payload     coingecko.MarketChartResponse `json:"payload"`
	LastUpdated time.Time                     `json:"lastUpdated"`
}

// GetHistory - respond to caller with historical exchange rates
func (s *Service) GetHistory(ctx context.Context, coinId CoingeckoCoin, vsCurrency CoingeckoVsCurrency, duration CoingeckoDuration) (*HistoryResponse, error) {
	// get logger from context
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		ctx, logger = logging.SetupLogger(ctx)
	}

	chart, err := s.coingecko.FetchMarketChart(ctx, coinId.String(), vsCurrency.String(), duration.ToDays())
	if err != nil {
		logger.Error().Err(err).Msg("failed to fetch chart from coingecko")
		return nil, fmt.Errorf("failed to fetch chart from coingecko: %w", err)
	}

	return &HistoryResponse{
		Payload:     *chart,
		LastUpdated: time.Now(),
	}, nil
}
