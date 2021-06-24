package serve

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	// needed for profiling
	_ "net/http/pprof"
	// re-using viper bind-env for wallet env variables
	_ "github.com/brave-intl/bat-go/cmd/wallets"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/grant"
	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/payment"
	"github.com/brave-intl/bat-go/promotion"
	"github.com/brave-intl/bat-go/utils/clients"
	"github.com/brave-intl/bat-go/utils/clients/reputation"
	appctx "github.com/brave-intl/bat-go/utils/context"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/logging"
	srv "github.com/brave-intl/bat-go/utils/service"
	"github.com/brave-intl/bat-go/wallet"
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
	cmd.ServeCmd.AddCommand(GrantServerCmd)

	flagBuilder := cmd.NewFlagBuilder(GrantServerCmd)

	flagBuilder.Flag().Bool("enable-job-workers", true,
		"enable job workers (defaults true)").
		Bind("enable-job-workers").
		Env("ENABLE_JOB_WORKERS")

	flagBuilder.Flag().StringSlice("brave-transfer-promotion-ids", []string{""},
		"brave vg deposit destination promotion id").
		Bind("brave-transfer-promotion-ids").
		Env("BRAVE_TRANSFER_PROMOTION_IDS")

	flagBuilder.Flag().StringSlice("free-trial-skus", []string{""},
		"the list of free trial skus").
		Bind("free-trial-skus").
		Env("FREE_TRIAL_SKUS")

	flagBuilder.Flag().String("wallet-on-platform-prior-to", "",
		"wallet on platform prior to for transfer").
		Bind("wallet-on-platform-prior-to").
		Env("WALLET_ON_PLATFORM_PRIOR_TO")

	flagBuilder.Flag().Bool("reputation-on-drain", false,
		"check wallet reputation on drain").
		Bind("reputation-on-drain").
		Env("REPUTATION_ON_DRAIN")

	// stripe configurations
	flagBuilder.Flag().Bool("stripe-enabled", false,
		"is stripe enabled for payments").
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
}

func setupRouter(ctx context.Context, logger *zerolog.Logger) (context.Context, *chi.Mux, *promotion.Service, []srv.Job) {
	buildTime := ctx.Value(appctx.BuildTimeCTXKey).(string)
	commit := ctx.Value(appctx.CommitCTXKey).(string)
	version := ctx.Value(appctx.VersionCTXKey).(string)
	env := ctx.Value(appctx.EnvironmentCTXKey).(string)

	// runnable jobs for the services created
	jobs := []srv.Job{}

	govalidator.SetFieldsRequiredByDefault(true)

	r := chi.NewRouter()

	// chain should be:
	// id / transfer -> ip -> heartbeat -> request logger / recovery -> token check -> rate limit
	// -> instrumentation -> handler
	r.Use(chiware.RequestID)
	r.Use(middleware.RequestIDTransfer)

	// NOTE: This uses standard fowarding headers, note that this puts implicit trust in the header values
	// provided to us. In particular it uses the first element.
	// Consequently we should consider the request IP as primarily "informational".
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
		r.Use(middleware.RateLimiter(ctx, 180))
	}

	var walletService *wallet.Service
	// use cobra configurations for setting up wallet service
	// this way we can have the wallet service completely separated from
	// grants service and easily deployable.
	r, ctx, walletService = wallet.SetupService(ctx, r)

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

	paymentPG, err := payment.NewPostgres("", true, "payment_db")
	if err != nil {
		sentry.CaptureException(err)
		logger.Panic().Err(err).Msg("Must be able to init postgres connection to start")
	}

	paymentService, err := payment.InitService(ctx, paymentPG, walletService)
	if err != nil {
		sentry.CaptureException(err)
		logger.Panic().Err(err).Msg("Payment service initialization failed")
	}

	// add runnable jobs:
	jobs = append(jobs, paymentService.Jobs()...)

	r.Mount("/v1/credentials", payment.CredentialRouter(paymentService))
	r.Mount("/v1/orders", payment.Router(paymentService))
	// for payment webhook integrations
	r.Mount("/v1/webhooks", payment.WebhookRouter(paymentService))
	r.Mount("/v1/votes", payment.VoteRouter(paymentService))

	if os.Getenv("FEATURE_MERCHANT") != "" {
		payment.InitEncryptionKeys()
		paymentDB, err := payment.NewPostgres("", true, "merch_payment_db")
		if err != nil {
			sentry.CaptureException(err)
			logger.Panic().Err(err).Msg("Must be able to init postgres connection to start")
		}
		paymentService, err := payment.InitService(ctx, paymentDB, walletService)
		if err != nil {
			sentry.CaptureException(err)
			logger.Panic().Err(err).Msg("Payment service initialization failed")
		}
		r.Mount("/v1/merchants", payment.MerchantRouter(paymentService))
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

	r.Get("/health-check", handlers.HealthCheckHandler(version, buildTime, commit))

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
		// v3/captcha
		r.Mount("/v3/captcha", proxyRouter)
	}

	return ctx, r, promotionService, jobs
}

func jobWorker(ctx context.Context, job func(context.Context) (bool, error), duration time.Duration) {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		ctx, logger = logging.SetupLogger(ctx)
	}
	for {
		_, err := job(ctx)
		if err != nil {
			log := logger.Error().Err(err)
			httpError, ok := err.(*errorutils.ErrorBundle)
			if ok {
				state, ok := httpError.Data().(clients.HTTPState)
				if ok {
					log = log.Int("status", state.Status).
						Str("path", state.Path).
						Interface("data", state.Body)
				}
			}
			log.Msg("error encountered in job run")
			sentry.CaptureException(err)
		}
		// regardless if attempted or not, wait for the duration until retrying
		<-time.After(duration)
	}
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
	ctx = context.WithValue(ctx, appctx.BraveTransferPromotionIDCTXKey, viper.GetStringSlice("brave-transfer-promotion-ids"))
	ctx = context.WithValue(ctx, appctx.WalletOnPlatformPriorToCTXKey, viper.GetString("wallet-on-platform-prior-to"))
	ctx = context.WithValue(ctx, appctx.ReputationOnDrainCTXKey, viper.GetBool("reputation-on-drain"))

	// bitflyer variables
	ctx = context.WithValue(ctx, appctx.BitflyerExtraClientSecretCTXKey, viper.GetString("bitflyer-extra-client-secret"))
	ctx = context.WithValue(ctx, appctx.BitflyerClientSecretCTXKey, viper.GetString("bitflyer-client-secret"))
	ctx = context.WithValue(ctx, appctx.BitflyerClientIDCTXKey, viper.GetString("bitflyer-client-id"))

	// stripe variables
	ctx = context.WithValue(ctx, appctx.StripeEnabledCTXKey, viper.GetBool("stripe-enabled"))
	ctx = context.WithValue(ctx, appctx.StripeWebhookSecretCTXKey, viper.GetString("stripe-webhook-secret"))
	ctx = context.WithValue(ctx, appctx.StripeSecretCTXKey, viper.GetString("stripe-secret"))

	// free trial skus
	ctx = context.WithValue(ctx, appctx.FreeTrialSKUsCTXKey, viper.GetStringSlice("free-trial-skus"))

	ctx, r, _, jobs := setupRouter(ctx, logger)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if enableJobWorkers {
		for _, job := range jobs {
			// iterate over jobs
			for i := 0; i < job.Workers; i++ {
				// spin up a job worker for each worker
				logger.Debug().Msg("starting job worker")
				go jobWorker(ctx, job.Func, job.Cadence)
			}
		}
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
