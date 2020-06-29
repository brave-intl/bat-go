package context

import "errors"

// CTXKey - a type for context keys
type CTXKey string

const (
	// DatastoreCTXKey - the context key for getting the datastore
	DatastoreCTXKey CTXKey = "datastore"
	// RODatastoreCTXKey - the context key for getting the datastore
	RODatastoreCTXKey CTXKey = "ro_datastore"
	// PaginationOrderOptionsCTXKey - this is the pagination options context key
	PaginationOrderOptionsCTXKey CTXKey = "pagination_order_options"
	// ServiceKey - the key used for service context
	ServiceKey CTXKey = "service"
	// EnvironmentCTXKey - the key used for service context
	EnvironmentCTXKey CTXKey = "environment"
	// RatiosServerCTXKey - the context key for getting the ratios server
	RatiosServerCTXKey CTXKey = "ratios_server"
	// RatiosAccessTokenCTXKey - the context key for getting the ratios server access token
	RatiosAccessTokenCTXKey CTXKey = "ratios_access_token"
	// BaseCurrencyCTXKey - the context key for getting the default base currency
	BaseCurrencyCTXKey CTXKey = "base_currency"
	// DefaultMonthlyChoicesCTXKey - the context key for getting the default monthly choices
	DefaultMonthlyChoicesCTXKey CTXKey = "default_monthly_choices"
	// DefaultTipChoicesCTXKey - the context key for getting the default tip choices
	DefaultTipChoicesCTXKey CTXKey = "default_tip_choices"
	// DefaultACChoicesCTXKey - the context key for getting the default ac choices
	DefaultACChoicesCTXKey CTXKey = "default_ac_choices"
	// RatiosCacheExpiryDurationCTXKey - context key for ratios client cache expiry
	RatiosCacheExpiryDurationCTXKey CTXKey = "ratios_client_cache_expiry"
	// RatiosCachePurgeDurationCTXKey - context key for ratios client cache purge
	RatiosCachePurgeDurationCTXKey CTXKey = "ratios_client_cache_purge"
	// LedgerServiceCTXKey - the context key for getting the ledger server
	LedgerServiceCTXKey CTXKey = "ledger_service"
	// LedgerAccessTokenCTXKey - the context key for getting the ratios server access token
	LedgerAccessTokenCTXKey CTXKey = "ledger_access_token"
)

var (
	// ErrNotInContext - error you get when you ask for something not in the context.
	ErrNotInContext = errors.New("failed to get value from context")
	// ErrValueWrongType - error you get when you ask for something and it is not the type you expected
	ErrValueWrongType = errors.New("context value of wrong type")
)
