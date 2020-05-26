package context

import "errors"

// CTXKey - a type for context keys
type CTXKey string

const (
	// DatastoreCTXKey - the context key for getting the datastore
	DatastoreCTXKey CTXKey = "datastore"
	// ServiceKey - the key used for service context
	ServiceKey CTXKey = "service"
	// EnvironmentCTXKey - the key used for service context
	EnvironmentCTXKey CTXKey = "environment"
	// RatiosServerCTXKey - the context key for getting the ratios server
	RatiosServerCTXKey CTXKey = "ratios_server"
	// RatiosAccessTokenCTXKey - the context key for getting the ratios server access token
	RatiosAccessTokenCTXKey CTXKey = "ratios_access_token"
	// BaseCurrencyCTXKey - the context key for getting the ratios server access token
	BaseCurrencyCTXKey CTXKey = "base_currency"
	// DefaultMonthlyChoicesCTXKey - the context key for getting the ratios server access token
	DefaultMonthlyChoicesCTXKey CTXKey = "default_monthly_choices"
	// DefaultTipChoicesCTXKey - the context key for getting the ratios server access token
	DefaultTipChoicesCTXKey CTXKey = "default_tip_choices"
)

var (
	// ErrNotInContext - error you get when you ask for something not in the context.
	ErrNotInContext = errors.New("failed to get value from context")
	// ErrValueWrongType - error you get when you ask for something and it is not the type you expected
	ErrValueWrongType = errors.New("context value of wrong type")
)
