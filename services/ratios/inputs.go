package ratios

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	appctx "github.com/brave-intl/bat-go/libs/context"
)

// CoingeckoCoin - type for coingecko coin input
type CoingeckoCoin struct {
	input string
	coin  string
}

// String - stringer implmentation
func (cc *CoingeckoCoin) String() string {
	return string(cc.coin)
}

var (
	// ErrCoingeckoCoinEmpty - empty coin
	ErrCoingeckoCoinEmpty = errors.New("coin cannot be empty")
	// ErrCoingeckoCoinInvalid - indicates there is a validation issue with the coin
	ErrCoingeckoCoinInvalid = errors.New("invalid coin")
)

// Decode - implement decodable
func (cc *CoingeckoCoin) Decode(ctx context.Context, v []byte) error {
	idToSymbol := ctx.Value(appctx.CoingeckoIDToSymbolCTXKey).(map[string]string)
	symbolToID := ctx.Value(appctx.CoingeckoSymbolToIDCTXKey).(map[string]string)
	contractToID := ctx.Value(appctx.CoingeckoContractToIDCTXKey).(map[string]string)

	coin := strings.ToLower(string(v))
	if coin == "" {
		return ErrCoingeckoCoinEmpty
	}

	if _, ok := idToSymbol[coin]; ok {
		*cc = CoingeckoCoin{input: coin, coin: coin}
		return nil
	}

	if c, ok := symbolToID[coin]; ok {
		*cc = CoingeckoCoin{input: coin, coin: c}
		return nil
	}

	if c, ok := contractToID[coin]; ok {
		*cc = CoingeckoCoin{input: coin, coin: c}
		return nil
	}

	return nil
}

// Validate - implement validatable
func (cc *CoingeckoCoin) Validate(ctx context.Context) error {
	idToSymbol := ctx.Value(appctx.CoingeckoIDToSymbolCTXKey).(map[string]string)

	if _, ok := idToSymbol[cc.String()]; ok {
		return nil
	}
	return fmt.Errorf("%w: %s is not valid", ErrCoingeckoCoinInvalid, cc.String())
}

var (
	// ErrCoingeckoCoinListLimit - coin list limit
	ErrCoingeckoCoinListLimit = errors.New("too many coins passed")
)

// CoingeckoCoinList - type for coingecko coin list input
type CoingeckoCoinList []CoingeckoCoin

// String - stringer implmentation
func (ccl *CoingeckoCoinList) String() string {
	var s []string
	for _, coin := range *ccl {
		s = append(s, coin.String())
	}
	return strings.Join(s, ",")
}

// Decode - implement decodable
func (ccl *CoingeckoCoinList) Decode(ctx context.Context, v []byte) error {
	c := strings.Split(string(v), ",")
	coins := make([]CoingeckoCoin, len(c))

	for i, coin := range c {
		err := coins[i].Decode(ctx, []byte(coin))
		if err != nil {
			return err
		}
	}
	*ccl = CoingeckoCoinList(coins)
	return nil
}

// Validate - implement validatable
func (ccl *CoingeckoCoinList) Validate(ctx context.Context) error {
	coinLimit := ctx.Value(appctx.CoingeckoCoinLimitCTXKey).(int)
	if len(*ccl) > coinLimit {
		return fmt.Errorf("%w: %s is not valid", ErrCoingeckoCoinListLimit, ccl.String())
	}

	for _, coin := range *ccl {
		err := coin.Validate(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

// CoingeckoVsCurrency - type for coingecko vs currency input
type CoingeckoVsCurrency string

// String - stringer implmentation
func (cvc *CoingeckoVsCurrency) String() string {
	return string(*cvc)
}

var (
	// ErrCoingeckoVsCurrencyEmpty - empty coin
	ErrCoingeckoVsCurrencyEmpty = errors.New("vs currency cannot be empty")
	// ErrCoingeckoVsCurrencyInvalid - indicates there is a validation issue with the coin
	ErrCoingeckoVsCurrencyInvalid = errors.New("invalid vs currency")
)

// Decode - implement decodable
func (cvc *CoingeckoVsCurrency) Decode(ctx context.Context, v []byte) error {
	vc := strings.ToLower(string(v))
	if vc == "" {
		return ErrCoingeckoVsCurrencyEmpty
	}

	*cvc = CoingeckoVsCurrency(vc)
	return nil
}

// Validate - implement validatable
func (cvc *CoingeckoVsCurrency) Validate(ctx context.Context) error {
	supportedVsCurrencies := ctx.Value(appctx.CoingeckoSupportedVsCurrenciesCTXKey).(map[string]bool)

	if supportedVsCurrencies[cvc.String()] {
		return nil
	}
	return fmt.Errorf("%w: %s is not valid", ErrCoingeckoVsCurrencyInvalid, cvc.String())
}

var (
	// ErrCoingeckoVsCurrencyLimit - vs currency list limit
	ErrCoingeckoVsCurrencyLimit = errors.New("too many vs currencies passed")
)

// CoingeckoVsCurrencyList - type for coingecko vs currency list input
type CoingeckoVsCurrencyList []CoingeckoVsCurrency

// String - stringer implmentation
func (cvcl *CoingeckoVsCurrencyList) String() string {
	var s []string
	for _, vc := range *cvcl {
		s = append(s, vc.String())
	}
	return strings.Join(s, ",")
}

// Decode - implement decodable
func (cvcl *CoingeckoVsCurrencyList) Decode(ctx context.Context, v []byte) error {
	c := strings.Split(string(v), ",")
	currencies := make([]CoingeckoVsCurrency, len(c))

	for i, vc := range c {
		err := currencies[i].Decode(ctx, []byte(vc))
		if err != nil {
			return err
		}
	}
	*cvcl = CoingeckoVsCurrencyList(currencies)
	return nil
}

// Validate - implement validatable
func (cvcl *CoingeckoVsCurrencyList) Validate(ctx context.Context) error {
	vsCurrencyLimit := ctx.Value(appctx.CoingeckoVsCurrencyLimitCTXKey).(int)
	if len(*cvcl) > vsCurrencyLimit {
		return fmt.Errorf("%w: %s is not valid", ErrCoingeckoVsCurrencyLimit, cvcl.String())
	}

	for _, vc := range *cvcl {
		err := vc.Validate(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

// CoingeckoDuration - type for coingecko duration input
type CoingeckoDuration string

// String - stringer implmentation
func (cd *CoingeckoDuration) String() string {
	return string(*cd)
}

// ToDays - get duration in days
func (cd *CoingeckoDuration) ToDays() float32 {
	return durationDays[string(*cd)]
}

// ToGetHistoryCacheDurationSeconds returns the length of time a GetHistory
// cache entry is considered valid based on it's duration
func (cd *CoingeckoDuration) ToGetHistoryCacheDurationSeconds() int {
	var defaultCacheDurationSeconds, dayInSeconds int

	// Default cache duration seconds is calculated according to this formula:
	//
	// Cache time in seconds = 60 * 60 * (fraction of 1 day the duration is)
	//
	// E.g.
	//
	// 1h: 60 * 60 * 1/24 = 150 seconds
	// 1d: 60 * 60 * 24/24 = 3600 seconds (1 hour)
	// 1w: 60 * 60 * 7 = 25200 seconds (7 hours)
	// 1m: 60 * 60 * 30 = 108000 seconds (30 hours)
	// 3m: 60 * 60 * 90 = 324000 seconds (3.75 days)
	// 1y: 60 * 60 * 365 = 31536000 seconds (15.2 days)
	// all: 60 * 60 * 365*10 = 315360000 seconds (152 days)
	days := cd.ToDays()
	defaultCacheDurationSeconds = int(math.Round(float64(60 * 60 * days)))

	// For any duration that would have a default cache duration greater
	// than 1 day, use 1 day instead.
	dayInSeconds = 60 * 60 * 24
	if defaultCacheDurationSeconds > dayInSeconds {
		return dayInSeconds
	}
	return defaultCacheDurationSeconds
}

var (
	durationDays = map[string]float32{
		"live": 1.0 / 24,
		"1h":   1.0 / 24,
		"1d":   1,
		"1w":   7,
		"1m":   30,
		"3m":   3 * 30,
		"1y":   365,
		"all":  10 * 365,
	}

	// ErrCoingeckoDurationInvalid - indicates there is a validation issue with the duration
	ErrCoingeckoDurationInvalid = errors.New("invalid duration")

	// ErrCoingeckoLimitInvalid - indicates there is a validation issue with the Limit
	ErrCoingeckoLimitInvalid = errors.New("invalid limit")
)

// Decode - implement decodable
func (cd *CoingeckoDuration) Decode(ctx context.Context, v []byte) error {
	d := strings.ToLower(string(v))
	*cd = CoingeckoDuration(d)
	return nil
}

// Validate - implement validatable
func (cd *CoingeckoDuration) Validate(ctx context.Context) error {
	if _, ok := durationDays[cd.String()]; ok {
		return nil
	}
	return fmt.Errorf("%w: %s is not valid", ErrCoingeckoDurationInvalid, cd.String())
}

// CoingeckoLimit - type for number of results per page
// Note: we only will request the first page
type CoingeckoLimit int

// String - stringer implmentation
func (cl *CoingeckoLimit) String() string {
	return strconv.Itoa(int(*cl))
}

// Int - int conversion implmentation
func (cl *CoingeckoLimit) Int() int {
	return int(*cl)
}

// Decode - implement decodable
func (cl *CoingeckoLimit) Decode(ctx context.Context, v []byte) error {
	l, err := strconv.Atoi(string(v))
	if err != nil {
		return err
	}

	*cl = CoingeckoLimit(l)
	return nil
}

// Validate - implement validatable
func (cl *CoingeckoLimit) Validate(ctx context.Context) error {
	if !(0 < cl.Int() && cl.Int() <= 250) {
		return fmt.Errorf("%w: %s is not valid", ErrCoingeckoLimitInvalid, cl.String())
	}
	return nil
}
