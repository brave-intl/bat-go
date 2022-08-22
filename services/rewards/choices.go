package rewards

import (
	"context"

	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/shopspring/decimal"
)

var (
	// the default choices are derived by the formula using
	// the two tables below.

	// example:
	// if BAT to USD ratio is 0.2
	// lookup the index of the price increments that matches
	// use that index number as the choice list from
	// the choice table as the "default" autocontribute choices
	// returned in the rewards parameters
	choiceTable = [][]float64{
		{3, 5, 7, 10, 20},
		{4, 6, 9, 12, 25},
		{5, 8, 11, 17, 35},
		{6, 10, 14, 20, 40},
		{9, 12, 20, 35, 50},
		{15, 25, 35, 50, 100},
		{20, 35, 50, 85, 120},
		{30, 50, 70, 100, 120},
	}
	priceIncrements = []float64{
		1, 0.8, 0.6, 0.5, 0.35, 0.2, 0.15, 0.1,
	}
	defaultTipChoices = []float64{
		1, 10, 100,
	}
	defaultMonthlyChoices = []float64{
		1, 10, 100,
	}
)

func getChoices(ctx context.Context, ratio decimal.Decimal) []float64 {
	// get logger from context
	logger := logging.Logger(ctx, "rewards.getChoices")

	// if we have DefaultACChoices in the context, just return that.
	if acChoices, ok := ctx.Value(appctx.DefaultACChoicesCTXKey).([]float64); ok {
		return acChoices
	}

	// find the price increment given our ratio
	var (
		index = -1
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
	if index < 0 || index >= len(priceIncrements) {
		// use the last index if no matches
		index = len(priceIncrements) - 1
	}
	return choiceTable[index]
}

func getTipChoices(ctx context.Context) []float64 {
	c, ok := ctx.Value(appctx.DefaultTipChoicesCTXKey).([]float64)
	if !ok {
		return defaultTipChoices
	}
	return c
}

func getDefaultChoice(ctx context.Context) float64 {
	c, ok := ctx.Value(appctx.DefaultACChoiceCTXKey).(float64)
	if !ok {
		return 0
	}
	return c
}

func getMonthlyChoices(ctx context.Context) []float64 {
	c, ok := ctx.Value(appctx.DefaultMonthlyChoicesCTXKey).([]float64)
	if !ok {
		return defaultMonthlyChoices
	}
	return c
}
