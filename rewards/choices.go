package rewards

import (
	"context"

	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/shopspring/decimal"
)

var (
	choiceTable = [][]float64{
		{3, 5, 7, 10, 20},
		{4, 6, 9, 12, 25},
		{5, 8, 11, 17, 35},
		{6, 10, 14, 20, 40},
		{9, 12, 20, 35, 50},
		{15, 25, 35, 50, 100},
		{20, 35, 50, 85},
		{30, 50, 70, 100},
	}
	priceIncrements = []float64{
		1, 0.8, 0.6, 0.5, 0.35, 0.2, 0.15, 0.1,
	}
)

func getChoices(ctx context.Context, ratio decimal.Decimal) []float64 {
	// get logger from context
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		_, logger = logging.SetupLogger(ctx)
	}

	// find the price increment given our ratio
	var (
		index int = -1
	)

	var rate, exact = ratio.Float64()
	if !exact {
		logger.Debug().
			Float64("rate", rate).
			Msg("rate was not exact, down sampled to float64")
	}

	for i, v := range priceIncrements {
		if v <= rate {
			// this is the index we want from our choice table
			index = i
			break
		}
	}
	if index < 0 || index > len(priceIncrements)-1 {
		// use the last index if no matches
		index = len(priceIncrements) - 1
	}
	return choiceTable[index]
}

func getTipChoices(ctx context.Context) []float64 {
	return getDefaultChoices(ctx, appctx.DefaultTipChoicesCTXKey)
}

func getMonthlyChoices(ctx context.Context) []float64 {
	return getDefaultChoices(ctx, appctx.DefaultMonthlyChoicesCTXKey)
}

func getDefaultChoices(ctx context.Context, key appctx.CTXKey) []float64 {
	tipChoices, ok := ctx.Value(key).([]float64)
	if ok {
		return tipChoices
	}
	return defaultTipChoices
}

var (
	defaultTipChoices = []float64{
		1, 10, 100,
	}
	defaultMonthlyChoices = []float64{
		1, 10, 100,
	}
)
