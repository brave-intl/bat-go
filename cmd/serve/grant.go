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
	"github.com/brave-intl/bat-go/utils/clients/reputation"
	appctx "github.com/brave-intl/bat-go/utils/context"
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

	GrantServerCmd.PersistentFlags().Bool("enable-job-workers", true, "enable job workers (defaults true)")
	cmd.Must(viper.BindPFlag("enable-job-workers", GrantServerCmd.PersistentFlags().Lookup("enable-job-workers")))
	cmd.Must(viper.BindEnv("enable-job-workers", "ENABLE_JOB_WORKERS"))

	GrantServerCmd.PersistentFlags().StringSlice("brave-transfer-promotion-ids", []string{""}, "brave vg deposit destination promotion id")
	cmd.Must(viper.BindPFlag("brave-transfer-promotion-ids", GrantServerCmd.PersistentFlags().Lookup("brave-transfer-promotion-ids")))
	cmd.Must(viper.BindEnv("brave-transfer-promotion-ids", "BRAVE_TRANSFER_PROMOTION_IDS"))

	GrantServerCmd.PersistentFlags().StringSlice("wallet-on-platform-prior-to", []string{""}, "wallet on platform prior to for transfer")
	cmd.Must(viper.BindPFlag("wallet-on-platform-prior-to", GrantServerCmd.PersistentFlags().Lookup("wallet-on-platform-prior-to")))
	cmd.Must(viper.BindEnv("wallet-on-platform-prior-to", "WALLET_ON_PLATFORM_PRIOR_TO"))
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
	// (e.g. with header "X-Forwarded-For: client, proxy1, proxy2" it would yield "client" as the real IP.)
	// The grant server is only accessed by the ledger service, so headers are semi-trusted.
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
	r.Use(middleware.RateLimiter(ctx, 180))

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

	r.Get("/metrics", middleware.Metrics())

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
			logger.Error().Err(err).Msg("error encountered in job run")
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
	ctx = context.WithValue(ctx, appctx.BraveTransferPromotionIDCTXKey, viper.GetString("brave-transfer-promotion-ids"))
	ctx = context.WithValue(ctx, appctx.WalletOnPlatformPriorToCTXKey, viper.GetString("wallet-on-platform-prior-to"))

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
