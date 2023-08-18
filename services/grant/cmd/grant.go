package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"time"

	cmdutils "github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/libs/clients/bitflyer"

	// needed for profiling
	_ "net/http/pprof"
	// re-using viper bind-env for wallet env variables
	_ "github.com/brave-intl/bat-go/services/wallet/cmd"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/libs/clients/gemini"
	"github.com/brave-intl/bat-go/libs/clients/reputation"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/middleware"
	srv "github.com/brave-intl/bat-go/libs/service"
	servicescmd "github.com/brave-intl/bat-go/services/cmd"
	"github.com/brave-intl/bat-go/services/grant"
	"github.com/brave-intl/bat-go/services/promotion"
	"github.com/brave-intl/bat-go/services/skus"
	"github.com/brave-intl/bat-go/services/skus/storage/repository"
	"github.com/brave-intl/bat-go/services/wallet"
	sentry "github.com/getsentry/sentry-go"
	"github.com/go-chi/chi"
	chiware "github.com/go-chi/chi/middleware"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// GrantServerCmd start up the grant server
	GrantServerCmd = &cobra.Command{
		Use:   "grant",
		Short: "subcommand to start up grant server",
		Run:   cmd.Perform("grant", RunGrantServer),
	}
)

func init() {
	servicescmd.ServeCmd.AddCommand(GrantServerCmd)

	flagBuilder := cmdutils.NewFlagBuilder(GrantServerCmd)

	flagBuilder.Flag().Bool("require-uphold-destination-country", false,
		"require responses for linkings to uphold to contain country identity information").
		Bind("require-uphold-destination-country").
		Env("REQUIRE_UPHOLD_DESTINATION_COUNTRY")

	flagBuilder.Flag().Bool("enable-job-workers", true,
		"enable job workers (defaults true)").
		Bind("enable-job-workers").
		Env("ENABLE_JOB_WORKERS")

	flagBuilder.Flag().Bool("disable-disconnect", false,
		"disable custodian ability to disconnect rewards wallets").
		Bind("disable-disconnect").
		Env("DISABLE_DISCONNECT")

	flagBuilder.Flag().Bool("disable-uphold-linking", false,
		"disable custodian linking to uphold").
		Bind("disable-uphold-linking").
		Env("DISABLE_UPHOLD_LINKING")

	flagBuilder.Flag().Bool("disable-gemini-linking", false,
		"disable custodian linking to gemini").
		Bind("disable-gemini-linking").
		Env("DISABLE_GEMINI_LINKING")

	flagBuilder.Flag().Bool("disable-bitflyer-linking", false,
		"disable custodian linking to bitflyer").
		Bind("disable-bitflyer-linking").
		Env("DISABLE_BITFLYER_LINKING")

	flagBuilder.Flag().Bool("disable-zebpay-linking", false,
		"disable custodial linking for zebpay").
		Bind("disable-zebpay-linking").
		Env("DISABLE_ZEBPAY_LINKING")

	flagBuilder.Flag().StringSlice("brave-transfer-promotion-ids", []string{""},
		"brave vg deposit destination promotion id").
		Bind("brave-transfer-promotion-ids").
		Env("BRAVE_TRANSFER_PROMOTION_IDS")

	flagBuilder.Flag().StringSlice("skus-whitelist", []string{""},
		"the whitelist of skus").
		Bind("skus-whitelist").
		Env("SKUS_WHITELIST")

	flagBuilder.Flag().StringSlice("country-blacklist", []string{""},
		"the blacklist of countries for wallet linking").
		Bind("country-blacklist").
		Env("COUNTRY_BLACKLIST")

	flagBuilder.Flag().String("merge-param-bucket", "",
		"parameters for the application").
		Bind("merge-param-bucket").
		Env("MERGE_PARAM_BUCKET")

	flagBuilder.Flag().String("disabled-wallet-geo-countries", "disabled-wallet-geo-countries.json",
		"the json file containing disabled geo countries for wallet creation").
		Env("DISABLED_WALLET_GEO_COUNTRIES").
		Bind("disabled-wallet-geo-countries")

	flagBuilder.Flag().String("wallet-on-platform-prior-to", "",
		"wallet on platform prior to for transfer").
		Bind("wallet-on-platform-prior-to").
		Env("WALLET_ON_PLATFORM_PRIOR_TO")

	flagBuilder.Flag().Bool("reputation-on-drain", false,
		"check wallet reputation on drain").
		Bind("reputation-on-drain").
		Env("REPUTATION_ON_DRAIN")

	flagBuilder.Flag().Bool("use-custodian-regions", false,
		"use custodian regions for figuring block on country for linking").
		Bind("use-custodian-regions").
		Env("USE_CUSTODIAN_REGIONS")

	flagBuilder.Flag().Bool("reputation-withdrawal-on-drain", false,
		"check wallet withdrawal reputation on drain").
		Bind("reputation-withdrawal-on-drain").
		Env("REPUTATION_WITHDRAWAL_ON_DRAIN")

	// Configuration for Radom.
	flagBuilder.Flag().Bool(
		"radom-enabled",
		false,
		"is radom enabled for skus",
	).Bind("radom-enabled").Env("RADOM_ENABLED")

	flagBuilder.Flag().String(
		"radom-seller-address",
		"",
		"the seller address for radom",
	).Bind("radom-seller-address").Env("RADOM_SELLER_ADDRESS")

	flagBuilder.Flag().String(
		"radom-server",
		"",
		"the server address for radom",
	).Bind("radom-server").Env("RADOM_SERVER")

	flagBuilder.Flag().String(
		"radom-secret",
		"",
		"the server token for radom",
	).Bind("radom-secret").Env("RADOM_SECRET")

	flagBuilder.Flag().String(
		"radom-webhook-secret",
		"",
		"the server webhook secret for radom",
	).Bind("radom-webhook-secret").Env("RADOM_WEBHOOK_SECRET")

	// stripe configurations
	flagBuilder.Flag().Bool("stripe-enabled", false,
		"is stripe enabled for skus").
		Bind("stripe-enabled").
		Env("STRIPE_ENABLED")

	flagBuilder.Flag().String("stripe-webhook-secret", "",
		"the stripe webhook secret").
		Bind("stripe-webhook-secret").
		Env("STRIPE_WEBHOOK_SECRET")

	flagBuilder.Flag().String("stripe-secret", "",
		"the stripe secret").
		Bind("stripe-secret").
		Env("STRIPE_SECRET")

	// gemini skus credentials
	flagBuilder.Flag().String("skus-gemini-settlement-address", "",
		"the settlement address for skus gemini").
		Bind("skus-gemini-settlement-address").
		Env("SKUS_GEMINI_SETTLEMENT_ADDRESS")

	flagBuilder.Flag().String("skus-gemini-api-key", "",
		"the api key for skus gemini").
		Bind("skus-gemini-api-key").
		Env("SKUS_GEMINI_API_KEY")

	flagBuilder.Flag().String("skus-gemini-api-secret", "",
		"the api secret for skus gemini").
		Bind("skus-gemini-api-secret").
		Env("SKUS_GEMINI_API_SECRET")

	flagBuilder.Flag().String("skus-gemini-browser-client-id", "",
		"the browser client id for gemini, which is the oauth client id the browser uses, required to validate transactions for AC flow").
		Bind("skus-gemini-browser-client-id").
		Env("SKUS_GEMINI_BROWSER_CLIENT_ID")

	flagBuilder.Flag().String("skus-gemini-client-id", "",
		"the client id for skus gemini").
		Bind("skus-gemini-client-id").
		Env("SKUS_GEMINI_CLIENT_ID")

	flagBuilder.Flag().String("skus-gemini-client-secret", "",
		"the client secret for skus gemini").
		Bind("skus-gemini-client-secret").
		Env("SKUS_GEMINI_CLIENT_SECRET")

	// gemini credentials
	flagBuilder.Flag().String("gemini-settlement-address", "",
		"the settlement address for gemini").
		Bind("gemini-settlement-address").
		Env("GEMINI_SETTLEMENT_ADDRESS")

	flagBuilder.Flag().String("gemini-api-key", "",
		"the api key for gemini").
		Bind("gemini-api-key").
		Env("GEMINI_API_KEY")

	flagBuilder.Flag().String("gemini-api-secret", "",
		"the api secret for gemini").
		Bind("gemini-api-secret").
		Env("GEMINI_API_SECRET")

	flagBuilder.Flag().String("gemini-browser-client-id", "",
		"the browser client id for gemini, which is the oauth client id the browser uses, required to validate transactions for AC flow").
		Bind("gemini-browser-client-id").
		Env("GEMINI_BROWSER_CLIENT_ID")

	flagBuilder.Flag().String("gemini-client-id", "",
		"the client id for gemini").
		Bind("gemini-client-id").
		Env("GEMINI_CLIENT_ID")

	flagBuilder.Flag().String("gemini-client-secret", "",
		"the client secret for gemini").
		Bind("gemini-client-secret").
		Env("GEMINI_CLIENT_SECRET")

	flagBuilder.Flag().String("zebpay-linking-key", "",
		"the linking key for zebpay custodian").
		Bind("zebpay-linking-key").
		Env("ZEBPAY_LINKING_KEY")

	// bitflyer credentials
	flagBuilder.Flag().String("bitflyer-client-id", "",
		"tells bitflyer what the client id is during token generation").
		Bind("bitflyer-client-id").
		Env("BITFLYER_CLIENT_ID")

	flagBuilder.Flag().String("bitflyer-client-secret", "",
		"tells bitflyer what the client secret during token generation").
		Bind("bitflyer-client-secret").
		Env("BITFLYER_CLIENT_SECRET")

	flagBuilder.Flag().String("bitflyer-extra-client-secret", "",
		"tells bitflyer what the extra client secret is during token").
		Bind("bitflyer-extra-client-secret").
		Env("BITFLYER_EXTRA_CLIENT_SECRET")

	flagBuilder.Flag().String("bitflyer-server", "",
		"the bitflyer domain to interact with").
		Bind("bitflyer-server").
		Env("BITFLYER_SERVER")

	flagBuilder.Flag().String("unlinking-cooldown", "",
		"the cooldown period for custodial wallet unlinking").
		Bind("unlinking-cooldown").
		Env("UNLINKING_COOLDOWN")

	// playstore json key
	flagBuilder.Flag().String("playstore-json-key", "",
		"the playstore json key").
		Bind("playstore-json-key").
		Env("PLAYSTORE_JSON_KEY")

	// appstore key
	flagBuilder.Flag().String("apple-receipt-shared-key", "",
		"the appstore shared key").
		Bind("apple-receipt-shared-key").
		Env("APPLE_RECEIPT_SHARED_KEY")

	flagBuilder.Flag().Bool("enable-store-signed-order-creds-consumer", true,
		"enable store signed order creds consumer").
		Bind("enable-store-signed-order-creds-consumer").
		Env("ENABLE_STORE_SIGNED_ORDER_CREDS_CONSUMER")

	flagBuilder.Flag().Int("number-store-signed-order-creds-consumer", 1,
		"number of consumers to create for store signed order creds").
		Bind("number-store-signed-order-creds-consumer").
		Env("NUMBER_STORE_SIGNED_ORDER_CREDS_CONSUMER")

	flagBuilder.Flag().String("kafka-brokers", "",
		"kafka broker list").
		Bind("kafka-brokers").
		Env("KAFKA_BROKERS")
}

func setupRouter(ctx context.Context, logger *zerolog.Logger) (context.Context, *chi.Mux, *promotion.Service, []srv.Job) {
	buildTime, _ := ctx.Value(appctx.BuildTimeCTXKey).(string)
	commit, _ := ctx.Value(appctx.CommitCTXKey).(string)
	version, _ := ctx.Value(appctx.VersionCTXKey).(string)
	env, _ := ctx.Value(appctx.EnvironmentCTXKey).(string)

	// runnable jobs for the services created
	jobs := []srv.Job{}

	govalidator.SetFieldsRequiredByDefault(true)

	r := chi.NewRouter()

	// chain should be:
	// id / transfer -> ip -> heartbeat -> request logger / recovery -> token check -> rate limit
	// -> instrumentation -> handler
	r.Use(chiware.RequestID)
	r.Use(middleware.RequestIDTransfer)
	r.Use(middleware.HostTransfer)

	// NOTE: This uses standard forwarding headers, note that this puts implicit trust in the header values
	// provided to us. In particular, it uses the first element.
	// Consequently, we should consider the request IP as primarily "informational".
	r.Use(chiware.RealIP)

	r.Use(chiware.Heartbeat("/"))
	// log and recover here
	if logger != nil {
		// Also handles panic recovery
		r.Use(hlog.NewHandler(*logger))
		r.Use(hlog.UserAgentHandler("user_agent"))
		r.Use(hlog.RequestIDHandler("req_id", "Request-Id"))
		r.Use(middleware.RequestLogger(logger))
	}
	// now we have middlewares we want included in logging
	r.Use(chiware.Timeout(15 * time.Second))
	r.Use(middleware.BearerToken)
	if os.Getenv("ENV") == "production" {
		// allow a burst of 4
		ctx = context.WithValue(ctx, appctx.RateLimiterBurstCTXKey, 4)
		// one request (or burst) every 500 ms
		r.Use(middleware.RateLimiter(ctx, 120))
	}

	var walletService *wallet.Service
	// use cobra configurations for setting up wallet service
	// this way we can have the wallet service completely separated from
	// grants service and easily deployable.
	ctx, walletService = wallet.SetupService(ctx)
	r = wallet.RegisterRoutes(ctx, walletService, r)

	promotionDB, promotionRODB, err := promotion.NewPostgres()
	if err != nil {
		logger.Panic().Err(err).Msg("unable connect to promotion db")
	}

	promotionService, err := promotion.InitService(
		ctx,
		promotionDB,
		promotionRODB,
		walletService,
	)
	if err != nil {
		sentry.CaptureException(err)
		logger.Panic().Err(err).Msg("Promotion service initialization failed")
	}

	grantDB, grantRODB, err := grant.NewPostgres()
	if err != nil {
		logger.Panic().Err(err).Msg("unable connect to grant db")
	}

	grantService, err := grant.InitService(
		ctx,
		grantDB,
		grantRODB,
		walletService,
		promotionService,
	)
	if err != nil {
		sentry.CaptureException(err)
		logger.Panic().Err(err).Msg("Grant service initialization failed")
	}

	// add runnable jobs:
	jobs = append(jobs, grantService.Jobs()...)

	// add runnable jobs:
	jobs = append(jobs, promotionService.Jobs()...)

	r.Mount("/v1/promotions", promotion.Router(promotionService))
	r.Mount("/v2/promotions", promotion.RouterV2(promotionService))

	sRouter, err := promotion.SuggestionsRouter(promotionService)
	if err != nil {
		logger.Panic().Err(err).Msg("failed to initialize the suggestions router")
	}

	r.Mount("/v1/suggestions", sRouter)

	sV2Router, err := promotion.SuggestionsV2Router(promotionService)
	if err != nil {
		logger.Panic().Err(err).Msg("failed to initialize the suggestions router")
	}

	r.Mount("/v2/suggestions", sV2Router)

	// temporarily house batloss events in promotion to avoid widespread conflicts later
	r.Mount("/v1/wallets", promotion.WalletEventRouter(promotionService))

	skuOrderRepo := repository.NewOrder()
	skuOrderItemRepo := repository.NewOrderItem()
	skuOrderPayHistRepo := repository.NewOrderPayHistory()

	skusPG, err := skus.NewPostgres(skuOrderRepo, skuOrderItemRepo, skuOrderPayHistRepo, "", true, "skus_db")
	if err != nil {
		sentry.CaptureException(err)
		logger.Panic().Err(err).Msg("Must be able to init postgres connection to start")
	}

	// skus gemini variables
	skuCtx := context.WithValue(ctx, appctx.GeminiSettlementAddressCTXKey, viper.GetString("skus-gemini-settlement-address"))
	skuCtx = context.WithValue(skuCtx, appctx.GeminiAPIKeyCTXKey, viper.GetString("skus-gemini-api-key"))
	skuCtx = context.WithValue(skuCtx, appctx.GeminiAPISecretCTXKey, viper.GetString("skus-gemini-api-secret"))
	skuCtx = context.WithValue(skuCtx, appctx.GeminiBrowserClientIDCTXKey, viper.GetString("skus-gemini-browser-client-id"))
	skuCtx = context.WithValue(skuCtx, appctx.GeminiClientIDCTXKey, viper.GetString("skus-gemini-client-id"))
	skuCtx = context.WithValue(skuCtx, appctx.GeminiClientSecretCTXKey, viper.GetString("skus-gemini-client-secret"))

	skusService, err := skus.InitService(skuCtx, skusPG, walletService)
	if err != nil {
		sentry.CaptureException(err)
		logger.Panic().Err(err).Msg("SKUs service initialization failed")
	}

	// add runnable jobs:
	jobs = append(jobs, skusService.Jobs()...)

	// initialize skus service keys for credentials to use
	skus.InitEncryptionKeys()

	r.Mount("/v1/credentials", skus.CredentialRouter(skusService))
	r.Mount("/v2/credentials", skus.CredentialV2Router(skusService))
	r.Mount("/v1/orders", skus.Router(skusService, middleware.InstrumentHandler))
	// for skus webhook integrations
	r.Mount("/v1/webhooks", skus.WebhookRouter(skusService))
	r.Mount("/v1/votes", skus.VoteRouter(skusService, middleware.InstrumentHandler))

	if os.Getenv("FEATURE_MERCHANT") != "" {
		skusDB, err := skus.NewPostgres(skuOrderRepo, skuOrderItemRepo, skuOrderPayHistRepo, "", true, "merch_skus_db")
		if err != nil {
			sentry.CaptureException(err)
			logger.Panic().Err(err).Msg("Must be able to init postgres connection to start")
		}

		skusService, err := skus.InitService(ctx, skusDB, walletService)
		if err != nil {
			sentry.CaptureException(err)
			logger.Panic().Err(err).Msg("SKUs service initialization failed")
		}
		r.Mount("/v1/merchants", skus.MerchantRouter(skusService))
	}

	// add profiling flag to enable profiling routes
	if os.Getenv("PPROF_ENABLED") != "" {
		// pprof attaches routes to default serve mux
		// host:6061/debug/pprof/
		go func() {
			log.Error().Err(http.ListenAndServe(":6061", http.DefaultServeMux))
		}()
	}

	logger.Info().
		Str("version", version).
		Str("commit", commit).
		Str("buildTime", buildTime).
		Msg("server starting up")

	{
		status := newSrvStatusFromCtx(ctx)
		r.Get("/health-check", handlers.HealthCheckHandler(version, buildTime, commit, status, nil))
	}

	reputationServer := os.Getenv("REPUTATION_SERVER")
	reputationToken := os.Getenv("REPUTATION_TOKEN")
	if len(reputationServer) == 0 {
		if env != "local" {
			logger.Panic().Msg("REPUTATION_SERVER is missing in production environment")
		}
	} else {
		proxyRouter := reputation.ProxyRouter(reputationServer, reputationToken)
		r.Mount("/v1/devicecheck", proxyRouter)
		r.Mount("/v1/captchas", proxyRouter)
		r.Mount("/v2/attestations/safetynet", proxyRouter)
		r.Mount("/v1/attestations/android", proxyRouter)
		// v3/captcha
		r.Mount("/v3/captcha", proxyRouter)
	}

	return ctx, r, promotionService, jobs
}

// RunGrantServer is the runner for starting up the grant server
func RunGrantServer(cmd *cobra.Command, args []string) error {
	enableJobWorkers, err := cmd.Flags().GetBool("enable-job-workers")
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	return GrantServer(
		ctx,
		enableJobWorkers,
	)
}

// GrantServer runs the grant server
func GrantServer(
	ctx context.Context,
	enableJobWorkers bool,
) error {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		ctx, logger = logging.SetupLogger(ctx)
	}

	sentryDsn := os.Getenv("SENTRY_DSN")
	if sentryDsn != "" {
		buildTime := ctx.Value(appctx.BuildTimeCTXKey).(string)
		commit := ctx.Value(appctx.CommitCTXKey).(string)
		err := sentry.Init(sentry.ClientOptions{
			Dsn:     sentryDsn,
			Release: fmt.Sprintf("bat-go@%s-%s", commit, buildTime),
		})
		defer sentry.Flush(2 * time.Second)
		if err != nil {
			logger.Panic().Err(err).Msg("unable to setup reporting!")
		}
	}
	logger.Info().
		Str("prefix", "main").
		Msg("Starting server")

	// add flags to context
	ctx = context.WithValue(ctx, appctx.KafkaBrokersCTXKey, viper.GetString("kafka-brokers"))
	ctx = context.WithValue(ctx, appctx.BraveTransferPromotionIDCTXKey, viper.GetStringSlice("brave-transfer-promotion-ids"))
	ctx = context.WithValue(ctx, appctx.WalletOnPlatformPriorToCTXKey, viper.GetString("wallet-on-platform-prior-to"))
	ctx = context.WithValue(ctx, appctx.ReputationOnDrainCTXKey, viper.GetBool("reputation-on-drain"))
	ctx = context.WithValue(ctx, appctx.UseCustodianRegionsCTXKey, viper.GetBool("use-custodian-regions"))
	ctx = context.WithValue(ctx, appctx.ReputationWithdrawalOnDrainCTXKey, viper.GetBool("reputation-withdrawal-on-drain"))
	// disable-disconnect wallet apis
	ctx = context.WithValue(ctx, appctx.DisableDisconnectCTXKey, viper.GetBool("disable-disconnect"))

	// bitflyer variables
	ctx = context.WithValue(ctx, appctx.BitflyerExtraClientSecretCTXKey, viper.GetString("bitflyer-extra-client-secret"))
	ctx = context.WithValue(ctx, appctx.BitflyerClientSecretCTXKey, viper.GetString("bitflyer-client-secret"))
	ctx = context.WithValue(ctx, appctx.BitflyerClientIDCTXKey, viper.GetString("bitflyer-client-id"))

	// gemini variables
	ctx = context.WithValue(ctx, appctx.GeminiSettlementAddressCTXKey, viper.GetString("gemini-settlement-address"))
	ctx = context.WithValue(ctx, appctx.GeminiAPIKeyCTXKey, viper.GetString("gemini-api-key"))
	ctx = context.WithValue(ctx, appctx.GeminiAPISecretCTXKey, viper.GetString("gemini-api-secret"))
	ctx = context.WithValue(ctx, appctx.GeminiBrowserClientIDCTXKey, viper.GetString("gemini-browser-client-id"))
	ctx = context.WithValue(ctx, appctx.GeminiClientIDCTXKey, viper.GetString("gemini-client-id"))
	ctx = context.WithValue(ctx, appctx.GeminiClientSecretCTXKey, viper.GetString("gemini-client-secret"))

	// zebpay wallet linking signing key
	ctx = context.WithValue(ctx, appctx.ZebPayLinkingKeyCTXKey, viper.GetString("zebpay-linking-key"))

	// linking variables
	ctx = context.WithValue(ctx, appctx.DisableZebPayLinkingCTXKey, viper.GetBool("disable-zebpay-linking"))
	ctx = context.WithValue(ctx, appctx.DisableUpholdLinkingCTXKey, viper.GetBool("disable-uphold-linking"))
	ctx = context.WithValue(ctx, appctx.DisableGeminiLinkingCTXKey, viper.GetBool("disable-gemini-linking"))
	ctx = context.WithValue(ctx, appctx.DisableBitflyerLinkingCTXKey, viper.GetBool("disable-bitflyer-linking"))

	// stripe variables
	ctx = context.WithValue(ctx, appctx.StripeEnabledCTXKey, viper.GetBool("stripe-enabled"))
	ctx = context.WithValue(ctx, appctx.StripeWebhookSecretCTXKey, viper.GetString("stripe-webhook-secret"))
	ctx = context.WithValue(ctx, appctx.StripeSecretCTXKey, viper.GetString("stripe-secret"))

	// Variables for Radom.
	ctx = context.WithValue(ctx, appctx.RadomEnabledCTXKey, viper.GetBool("radom-enabled"))
	ctx = context.WithValue(ctx, appctx.RadomWebhookSecretCTXKey, viper.GetString("radom-webhook-secret"))
	ctx = context.WithValue(ctx, appctx.RadomSecretCTXKey, viper.GetString("radom-secret"))
	ctx = context.WithValue(ctx, appctx.RadomServerCTXKey, viper.GetString("radom-server"))
	ctx = context.WithValue(ctx, appctx.RadomSellerAddressCTXKey, viper.GetString("radom-seller-address"))

	// require country present from uphold txs
	ctx = context.WithValue(ctx, appctx.RequireUpholdCountryCTXKey, viper.GetBool("require-uphold-destination-country"))

	// whitelisted skus
	ctx = context.WithValue(ctx, appctx.WhitelistSKUsCTXKey, viper.GetStringSlice("skus-whitelist"))

	// the bucket for the custodian regions
	ctx = context.WithValue(ctx, appctx.ParametersMergeBucketCTXKey, viper.Get("merge-param-bucket"))

	// the json file containing disabled wallet geo countries.
	ctx = context.WithValue(ctx, appctx.DisabledWalletGeoCountriesCTXKey, viper.Get("disabled-wallet-geo-countries"))

	// blacklisted countries
	ctx = context.WithValue(ctx, appctx.BlacklistedCountryCodesCTXKey, viper.GetStringSlice("country-blacklist"))

	// pull down from s3 the appropriate values for geo allow/block rules

	// custodian unlinking cooldown
	ctx = context.WithValue(ctx, appctx.NoUnlinkPriorToDurationCTXKey, viper.GetString("unlinking-cooldown"))

	// skus enable store signed order creds consumer
	ctx = context.WithValue(ctx, appctx.SkusEnableStoreSignedOrderCredsConsumer,
		viper.GetBool("enable-store-signed-order-creds-consumer"))

	// skus number of consumers to create for store signed order creds
	ctx = context.WithValue(ctx, appctx.SkusNumberStoreSignedOrderCredsConsumer,
		viper.GetInt("number-store-signed-order-creds-consumer"))

	// playstore json key
	// json key is base64
	jsonKey, err := base64.StdEncoding.DecodeString(viper.GetString("playstore-json-key"))
	if err != nil {
		logger.Error().Err(err).
			Msg("failed to decode the playstore json key")
	}
	ctx = context.WithValue(ctx, appctx.PlaystoreJSONKeyCTXKey, jsonKey)

	ctx = context.WithValue(ctx, appctx.AppleReceiptSharedKeyCTXKey, viper.GetString("apple-receipt-shared-key"))

	ctx, r, _, jobs := setupRouter(ctx, logger)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if enableJobWorkers {
		for _, job := range jobs {
			// iterate over jobs
			for i := 0; i < job.Workers; i++ {
				// spin up a job worker for each worker
				logger.Debug().Msg("starting job worker")
				go srv.JobWorker(ctx, job.Func, job.Cadence)
			}
		}
	}
	if viper.GetString("environment") != "local" &&
		viper.GetString("environment") != "development" {
		// run gemini balance watch so we have balance info in prometheus
		go func() {
			// no need to panic here, log the error and move on with serving
			if err := gemini.WatchGeminiBalance(ctx); err != nil {
				logger.Error().Err(err).Msg("error launching gemini balance watch")
			}
		}()
		go func() {
			if err := bitflyer.WatchBitflyerBalance(ctx, 2*time.Minute); err != nil {
				logger.Error().Err(err).Msg("error launching bitflyer balance watch")
			}
		}()
	}

	go func() {
		err := http.ListenAndServe(":9090", middleware.Metrics())
		if err != nil {
			sentry.CaptureException(err)
			logger.Panic().Err(err).Msg("metrics HTTP server start failed!")
		}
	}()

	srv := http.Server{
		Addr:         ":3333",
		Handler:      chi.ServerBaseContext(ctx, r),
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 20 * time.Second,
	}
	err = srv.ListenAndServe()
	if err != nil {
		sentry.CaptureException(err)
		logger.Panic().Err(err).Msg("HTTP server start failed!")
	}
	return nil
}

func newSrvStatusFromCtx(ctx context.Context) map[string]any {
	uh, _ := ctx.Value(appctx.DisableUpholdLinkingCTXKey).(bool)
	g, _ := ctx.Value(appctx.DisableGeminiLinkingCTXKey).(bool)
	bf, _ := ctx.Value(appctx.DisableBitflyerLinkingCTXKey).(bool)
	zp, _ := ctx.Value(appctx.DisableZebPayLinkingCTXKey).(bool)

	result := map[string]interface{}{
		"wallet": map[string]bool{
			"uphold":   !uh,
			"gemini":   !g,
			"bitflyer": !bf,
			"zebpay":   !zp,
		},
	}

	return result
}
