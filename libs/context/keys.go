package context

import "errors"

// CTXKey - a type for context keys
type CTXKey string

const (
	// MergeCustodialCTXKey - the context key for merge custodial
	MergeCustodialCTXKey CTXKey = "merge_custodial"
	// AWSClientCTXKey - the context key for an aws client
	AWSClientCTXKey CTXKey = "aws_client"
	// DatastoreCTXKey - the context key for getting the datastore
	DatastoreCTXKey CTXKey = "datastore"
	// DatabaseTransactionCTXKey - context key for database transactions
	DatabaseTransactionCTXKey CTXKey = "db_tx"
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
	// DefaultACChoiceCTXKey - the context key for getting the default ac choice
	DefaultACChoiceCTXKey CTXKey = "default_ac_choice"
	// RatiosCacheExpiryDurationCTXKey - context key for ratios client cache expiry
	RatiosCacheExpiryDurationCTXKey CTXKey = "ratios_client_cache_expiry"
	// RatiosCachePurgeDurationCTXKey - context key for ratios client cache purge
	RatiosCachePurgeDurationCTXKey CTXKey = "ratios_client_cache_purge"
	// DebugLoggingCTXKey - context key for debug logging
	DebugLoggingCTXKey CTXKey = "debug_logging"
	// ProgressLoggingCTXKey - context key for progress logging
	ProgressLoggingCTXKey CTXKey = "progress_logging"

	// VersionCTXKey - context key for version of code
	VersionCTXKey CTXKey = "version"
	// CommitCTXKey - context key for the commit of the code
	CommitCTXKey CTXKey = "commit"
	// BuildTimeCTXKey - context key for the build time of code
	BuildTimeCTXKey CTXKey = "build_time"
	// ReputationClientCTXKey - context key for the build time of code
	ReputationClientCTXKey CTXKey = "reputation_client"
	// GeminiClientCTXKey - context key for the build time of code
	GeminiClientCTXKey CTXKey = "gemini_client"
	// GeminiBrowserClientIDCTXKey - context key for the gemini browser client id
	GeminiBrowserClientIDCTXKey CTXKey = "gemini_browser_client_id"
	// GeminiClientIDCTXKey - context key for the gemini client id
	GeminiClientIDCTXKey CTXKey = "gemini_client_id"
	// GeminiClientSecretCTXKey - context key for the gemini client secret
	GeminiClientSecretCTXKey CTXKey = "gemini_client_secret"
	// GeminiAPIKeyCTXKey - context key for the gemini api key
	GeminiAPIKeyCTXKey CTXKey = "gemini_api_key"
	// GeminiAPISecretCTXKey - context key for the gemini api secret
	GeminiAPISecretCTXKey CTXKey = "gemini_api_secret"
	// GeminiSettlementAddressCTXKey - context key for the gemini settlement address
	GeminiSettlementAddressCTXKey CTXKey = "gemini_settlement_address"

	// for skus ac validation

	// SkusGeminiClientCTXKey - context key for the build time of code
	SkusGeminiClientCTXKey CTXKey = "skus_gemini_client"
	// SkusGeminiBrowserClientIDCTXKey - context key for the gemini browser client id
	SkusGeminiBrowserClientIDCTXKey CTXKey = "skus_gemini_browser_client_id"
	// SkusGeminiClientIDCTXKey - context key for the gemini client id
	SkusGeminiClientIDCTXKey CTXKey = "skus_gemini_client_id"
	// SkusGeminiClientSecretCTXKey - context key for the gemini client secret
	SkusGeminiClientSecretCTXKey CTXKey = "skus_gemini_client_secret"
	// SkusGeminiAPIKeyCTXKey - context key for the gemini api key
	SkusGeminiAPIKeyCTXKey CTXKey = "skus_gemini_api_key"
	// SkusGeminiAPISecretCTXKey - context key for the gemini api secret
	SkusGeminiAPISecretCTXKey CTXKey = "skus_gemini_api_secret"
	// SkusGeminiSettlementAddressCTXKey - context key for the gemini settlement address
	SkusGeminiSettlementAddressCTXKey CTXKey = "skus_gemini_settlement_address"

	// Kafka509CertCTXKey - context key for the build time of code
	Kafka509CertCTXKey CTXKey = "kafka_x509_cert"
	// KafkaBrokersCTXKey - context key for the build time of code
	KafkaBrokersCTXKey CTXKey = "kafka_brokers"
	// BraveTransferPromotionIDCTXKey - context key for the build time of code
	BraveTransferPromotionIDCTXKey CTXKey = "brave_transfer_promotion_id"
	// WalletOnPlatformPriorToCTXKey - context key for the build time of code
	WalletOnPlatformPriorToCTXKey CTXKey = "wallet_on_platform_prior_to"
	// LogLevelCTXKey - context key for application logging level
	LogLevelCTXKey CTXKey = "log_level"
	// BitFlyerJWTKeyCTXKey - context key for the bitflyer jwt key
	BitFlyerJWTKeyCTXKey CTXKey = "bitflyer_jwt_key"
	// BitflyerExtraClientSecretCTXKey - context key for the extra client secret
	BitflyerExtraClientSecretCTXKey CTXKey = "bitflyer_extra_client_secret"
	// BitflyerClientSecretCTXKey - context key for the client secret
	BitflyerClientSecretCTXKey CTXKey = "bitflyer_client_secret"
	// BitflyerClientIDCTXKey - context key for the client secret
	BitflyerClientIDCTXKey CTXKey = "bitflyer_client_id"
	// ReputationOnDrainCTXKey - context key for getting the reputation on drain feature flag
	ReputationOnDrainCTXKey CTXKey = "reputation_on_drain"
	// UseCustodianRegionsCTXKey - context key for getting the reputation on drain feature flag
	UseCustodianRegionsCTXKey CTXKey = "use-custodian-regions"
	// CustodianRegionsCTXKey - context key for getting the reputation on drain feature flag
	CustodianRegionsCTXKey CTXKey = "custodian-regions"
	// ReputationWithdrawalOnDrainCTXKey - context key for getting the reputation on drain feature flag
	ReputationWithdrawalOnDrainCTXKey CTXKey = "reputation_withdrawal_on_drain"
	// SkipRedeemCredentialsCTXKey - context key for getting the skip redeem credentials
	SkipRedeemCredentialsCTXKey CTXKey = "skip_redeem_credentials"

	// DisableUpholdLinkingCTXKey - this informs if uphold linking is enabled
	DisableUpholdLinkingCTXKey CTXKey = "disable_uphold_linking"
	// DisableGeminiLinkingCTXKey - this informs if gemini linking is enabled
	DisableGeminiLinkingCTXKey CTXKey = "disable_gemini_linking"
	// DisableBitflyerLinkingCTXKey - this informs if bitflyer linking is enabled
	DisableBitflyerLinkingCTXKey CTXKey = "disable_bitflyer_linking"

	// stripe related keys

	// StripeEnabledCTXKey - this informs if stripe is enabled
	StripeEnabledCTXKey CTXKey = "stripe_enabled"
	// StripeWebhookSecretCTXKey - the webhook secret key for stripe integration
	StripeWebhookSecretCTXKey CTXKey = "stripe_webhook_secret"
	// StripeSecretCTXKey - the secret key for stripe integration
	StripeSecretCTXKey CTXKey = "stripe_secret"
	// WhitelistSKUsCTXKey - context key for whitelisted skus
	WhitelistSKUsCTXKey CTXKey = "whitelist_skus"

	// RateLimiterBurstCTXKey - context key for allowing a bursting rate limiter
	RateLimiterBurstCTXKey CTXKey = "rate_limit_burst"
	// NoUnlinkPriorToDurationCTXKey - the iso duration of time that no unlinking must have happened
	NoUnlinkPriorToDurationCTXKey CTXKey = "no_unlinkings_prior_to"
	// CoingeckoServerCTXKey - the context key for getting the coingecko server
	CoingeckoServerCTXKey CTXKey = "coingecko_server"
	// CoingeckoAccessTokenCTXKey - the context key for getting the coingecko server access token
	CoingeckoAccessTokenCTXKey CTXKey = "coingecko_access_token"

	// CoingeckoIDToSymbolCTXKey - the context key for getting the mapping from coin id to symbol
	CoingeckoIDToSymbolCTXKey CTXKey = "coingecko_id_to_symbol"
	// CoingeckoSymbolToIDCTXKey - the context key for getting the mapping from coin symbol to id
	CoingeckoSymbolToIDCTXKey CTXKey = "coingecko_symbol_to_id"
	// CoingeckoContractToIDCTXKey - the context key for getting the mapping from coin contract to id
	CoingeckoContractToIDCTXKey CTXKey = "coingecko_contract_to_id"
	// CoingeckoSupportedVsCurrenciesCTXKey - the context key for getting the list of supported vs currencies
	CoingeckoSupportedVsCurrenciesCTXKey CTXKey = "coingecko_supported_vs_currencies"
	// CoingeckoCoinLimitCTXKey - the context key for getting the max number of coins
	CoingeckoCoinLimitCTXKey CTXKey = "coingecko_coin_limit"
	// CoingeckoVsCurrencyLimitCTXKey - the context key for getting the max number of vs currencies
	CoingeckoVsCurrencyLimitCTXKey CTXKey = "coingecko_vs_currency_limit"
	// RatiosRedisAddrCTXKey - the context key for getting the ratios redis address
	RatiosRedisAddrCTXKey CTXKey = "ratios_redis_addr"
	// BlacklistedCountryCodesCTXKey - the context key for getting the ratios redis address
	BlacklistedCountryCodesCTXKey CTXKey = "blacklisted_country_codes"

	// RateLimitPerMinuteCTXKey - the context key for getting the rate limit
	RateLimitPerMinuteCTXKey CTXKey = "rate_limit_per_min"

	// ParametersMergeBucketCTXKey - the context key for getting the rate limit
	ParametersMergeBucketCTXKey CTXKey = "merge_param_bucket"
	// AWSRegionCTXKey - the context key for getting the rate limit
	AWSRegionCTXKey CTXKey = "aws_region"
	// RequireUpholdCountryCTXKey - the context key for getting the rate limit
	RequireUpholdCountryCTXKey CTXKey = "require_uphold_country"

	// DisabledWalletGeoCountriesCTXKey context key used to retrieve the S3 object name for disabled wallet geo countries
	DisabledWalletGeoCountriesCTXKey CTXKey = "disabled_wallet_geo_countries"

	// PlaystoreJSONKeyCTXKey - the context key for playstore json key
	PlaystoreJSONKeyCTXKey CTXKey = "playstore_json_key"

	// AppleReceiptSharedKeyCTXKey - the context key for appstore key
	AppleReceiptSharedKeyCTXKey CTXKey = "apple_receipt_shared_key"

	// DisableDisconnectCTXKey - the context key for rewards wallet disconnect capability key
	DisableDisconnectCTXKey CTXKey = "disable_disconnect"

	// ParametersVBATDeadlineCTXKey - the context key for getting the vbat deadline
	ParametersVBATDeadlineCTXKey CTXKey = "parameters_vbat_deadline"
	// ParametersTransitionCTXKey - the context key for getting the vbat deadline
	ParametersTransitionCTXKey CTXKey = "parameters_transition"
)

var (
	// ErrNotInContext - error you get when you ask for something not in the context.
	ErrNotInContext = errors.New("failed to get value from context")
	// ErrValueWrongType - error you get when you ask for something, and it is not the type you expected
	ErrValueWrongType = errors.New("context value of wrong type")
)
