package main

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/controllers"
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
)

var (
	commit    string
	version   string
	buildTime string
)

func setupLogger(ctx context.Context) (context.Context, *zerolog.Logger) {
	return logging.SetupLogger(context.WithValue(ctx, appctx.EnvironmentCTXKey, os.Getenv("ENV")))
}

func setupRouter(ctx context.Context, logger *zerolog.Logger) (context.Context, *chi.Mux, *promotion.Service, []srv.Job) {

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
	r.Use(middleware.RateLimiter(ctx))

	var walletService *wallet.Service
	// use cobra configurations for setting up wallet service
	// this way we can have the wallet service completely separated from
	// grants service and easily deployable.
	r, ctx, walletService = cmd.SetupWalletService(ctx, r)

	promotionDB, promotionRODB, err := promotion.NewPostgres()
	if err != nil {
		log.Panic().Err(err).Msg("unable connect to promotion db")
	}
	promotionService, err := promotion.InitService(
		ctx,
		promotionDB,
		promotionRODB,
		walletService,
	)
	if err != nil {
		sentry.CaptureException(err)
		log.Panic().Err(err).Msg("Promotion service initialization failed")
	}

	grantDB, grantRODB, err := grant.NewPostgres()
	if err != nil {
		log.Panic().Err(err).Msg("unable connect to grant db")
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
		log.Panic().Err(err).Msg("Grant service initialization failed")
	}

	// add runnable jobs:
	jobs = append(jobs, grantService.Jobs()...)

	// add runnable jobs:
	jobs = append(jobs, promotionService.Jobs()...)

	r.Mount("/v1/grants", controllers.GrantsRouter(grantService))
	r.Mount("/v1/promotions", promotion.Router(promotionService))
	r.Mount("/v2/promotions", promotion.RouterV2(promotionService))

	sRouter, err := promotion.SuggestionsRouter(promotionService)
	if err != nil {
		log.Panic().Err(err).Msg("failed to initialize the suggestions router")
	}

	r.Mount("/v1/suggestions", sRouter)
	// temporarily house batloss events in promotion to avoid widespread conflicts later
	r.Mount("/v1/wallets", promotion.WalletEventRouter(promotionService))

	paymentPG, err := payment.NewPostgres("", true, "payment_db")
	if err != nil {
		sentry.CaptureException(err)
		log.Panic().Err(err).Msg("Must be able to init postgres connection to start")
	}
	paymentService, err := payment.InitService(ctx, paymentPG, walletService)
	if err != nil {
		sentry.CaptureException(err)
		log.Panic().Err(err).Msg("Payment service initialization failed")
	}

	// add runnable jobs:
	jobs = append(jobs, paymentService.Jobs()...)

	r.Mount("/v1/orders", payment.Router(paymentService))
	r.Mount("/v1/votes", payment.VoteRouter(paymentService))

	if os.Getenv("FEATURE_MERCHANT") != "" {
		payment.InitEncryptionKeys()
		paymentDB, err := payment.NewPostgres("", true, "merch_payment_db")
		if err != nil {
			sentry.CaptureException(err)
			log.Panic().Err(err).Msg("Must be able to init postgres connection to start")
		}
		paymentService, err := payment.InitService(ctx, paymentDB, walletService)
		if err != nil {
			sentry.CaptureException(err)
			log.Panic().Err(err).Msg("Payment service initialization failed")
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

	log.Info().
		Str("version", version).
		Str("commit", commit).
		Str("buildTime", buildTime).
		Msg("server starting up")

	r.Get("/health-check", handlers.HealthCheckHandler(version, buildTime, commit))

	env := os.Getenv("ENV")
	reputationServer := os.Getenv("REPUTATION_SERVER")
	reputationToken := os.Getenv("REPUTATION_TOKEN")
	if len(reputationServer) == 0 {
		if env != "local" {
			log.Panic().Msg("REPUTATION_SERVER is missing in production environment")
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
		ctx, logger = setupLogger(ctx)
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

func main() {
	var (
		serverCtx, logger = setupLogger(context.Background())
	)

	// setup sentry
	sentryDsn := os.Getenv("SENTRY_DSN")
	if sentryDsn != "" {
		err := sentry.Init(sentry.ClientOptions{
			Dsn:     sentryDsn,
			Release: fmt.Sprintf("bat-go@%s-%s", commit, buildTime),
		})
		defer sentry.Flush(2 * time.Second)
		if err != nil {
			logger.Panic().Err(err).Msg("unable to setup reporting!")
		}
	}
	subLog := logger.Info().Str("prefix", "main")
	subLog.Msg("Starting server")

	serverCtx, r, _, jobs := setupRouter(serverCtx, logger)

	serverCtx, cancel := context.WithCancel(serverCtx)
	defer cancel()

	if os.Getenv("ENABLE_JOB_WORKERS") != "" {
		for _, job := range jobs {
			// iterate over jobs
			for i := 0; i < job.Workers; i++ {
				// spin up a job worker for each worker
				logger.Debug().Msg("starting job worker")
				go jobWorker(serverCtx, job.Func, job.Cadence)
			}
		}
	}

	srv := http.Server{
		Addr:         ":3333",
		Handler:      chi.ServerBaseContext(serverCtx, r),
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 20 * time.Second,
	}
	err := srv.ListenAndServe()
	if err != nil {
		sentry.CaptureException(err)
		logger.Panic().Err(err).Msg("HTTP server start failed!")
	}
}
