package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/controllers"
	"github.com/brave-intl/bat-go/grant"
	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/payment"
	"github.com/brave-intl/bat-go/promotion"
	"github.com/brave-intl/bat-go/utils/clients/reputation"
	"github.com/brave-intl/bat-go/utils/handlers"
	srv "github.com/brave-intl/bat-go/utils/service"
	"github.com/getsentry/sentry-go"
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
	var output io.Writer
	if os.Getenv("ENV") != "local" {
		output = os.Stdout
	} else {
		output = zerolog.ConsoleWriter{Out: os.Stdout}
	}

	// always print out timestamp
	log := zerolog.New(output).With().Timestamp().Logger()

	debug := os.Getenv("DEBUG")
	if debug == "" || debug == "f" || debug == "n" || debug == "0" {
		log = log.Level(zerolog.InfoLevel)
	}

	return log.WithContext(ctx), &log
}

func setupRouter(ctx context.Context, logger *zerolog.Logger) (context.Context, chi.Router, *promotion.Service, []srv.Job) {

	// runnable jobs for the services created
	jobs := []srv.Job{}

	govalidator.SetFieldsRequiredByDefault(true)

	var r = chi.NewRouter().With()
	r.Use(chiware.RequestID)

	// NOTE: This uses standard fowarding headers, note that this puts implicit trust in the header values
	// provided to us. In particular it uses the first element.
	// (e.g. with header "X-Forwarded-For: client, proxy1, proxy2" it would yield "client" as the real IP.)
	// The grant server is only accessed by the ledger service, so headers are semi-trusted.
	// Consequently we should consider the request IP as primarily "informational".
	r.Use(chiware.RealIP)

	r.Use(chiware.Heartbeat("/"))
	r.Use(chiware.Timeout(60 * time.Second))
	r.Use(middleware.BearerToken)
	r.Use(middleware.RateLimiter)
	r.Use(middleware.RequestIDTransfer)
	if logger != nil {
		// Also handles panic recovery
		r.Use(hlog.NewHandler(*logger))
		r.Use(hlog.UserAgentHandler("user_agent"))
		r.Use(hlog.RequestIDHandler("req_id", "Request-Id"))
		r.Use(middleware.RequestLogger(logger))
	}

	r.Use(middleware.InstrumentHandler)

	roDB := os.Getenv("RO_DATABASE_URL")

	var grantRoPg grant.ReadOnlyDatastore
	grantPg, err := grant.NewPostgres("", true, "grant_db")
	if err != nil {
		sentry.CaptureMessage(err.Error())
		sentry.Flush(time.Second * 2)
		log.Panic().Err(err).Msg("Must be able to init postgres connection to start")
	}
	if len(roDB) > 0 {
		grantRoPg, err = grant.NewPostgres(roDB, false, "grant_read_only_db")
		if err != nil {
			sentry.CaptureMessage(err.Error())
			sentry.Flush(time.Second * 2)
			log.Error().Err(err).Msg("Could not start reader postgres connection")
		}
	}

	grantService, err := grant.InitService(grantPg, grantRoPg)
	if err != nil {
		sentry.CaptureMessage(err.Error())
		sentry.Flush(time.Second * 2)
		log.Panic().Err(err).Msg("Grant service initialization failed")
	}

	// add runnable jobs:
	jobs = append(jobs, grantService.Jobs()...)

	var roPg promotion.ReadOnlyDatastore
	pg, err := promotion.NewPostgres("", true, "promotion_db")
	if err != nil {
		sentry.CaptureMessage(err.Error())
		sentry.Flush(time.Second * 2)
		log.Panic().Err(err).Msg("Must be able to init postgres connection to start")
	}
	if len(roDB) > 0 {
		roPg, err = promotion.NewPostgres(roDB, false, "promotion_read_only_db")
		if err != nil {
			sentry.CaptureMessage(err.Error())
			sentry.Flush(time.Second * 2)
			log.Error().Err(err).Msg("Could not start reader postgres connection")
		}
	}

	promotionService, err := promotion.InitService(pg, roPg)
	if err != nil {
		sentry.CaptureMessage(err.Error())
		sentry.Flush(time.Second * 2)
		log.Panic().Err(err).Msg("Promotion service initialization failed")
	}

	// add runnable jobs:
	jobs = append(jobs, promotionService.Jobs()...)

	r.Mount("/v1/grants", controllers.GrantsRouter(grantService))
	r.Mount("/v1/promotions", promotion.Router(promotionService))
	r.Mount("/v1/suggestions", promotion.SuggestionsRouter(promotionService))

	if os.Getenv("FEATURE_ORDERS") != "" {
		paymentPG, err := payment.NewPostgres("", true, "payment_db")
		if err != nil {
			sentry.CaptureMessage(err.Error())
			sentry.Flush(time.Second * 2)
			log.Panic().Err(err).Msg("Must be able to init postgres connection to start")
		}
		paymentService, err := payment.InitService(paymentPG)
		if err != nil {
			sentry.CaptureMessage(err.Error())
			sentry.Flush(time.Second * 2)
			log.Panic().Err(err).Msg("Payment service initialization failed")
		}

		// add runnable jobs:
		jobs = append(jobs, paymentService.Jobs()...)

		r.Mount("/v1/orders", payment.Router(paymentService))
		r.Mount("/v1/votes", payment.VoteRouter(paymentService))
	}
	r.Get("/metrics", middleware.Metrics())

	log.Printf("server version/buildtime = %s %s %s", version, commit, buildTime)
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
	var attemptedCount = 0
	for {
		attempted, err := job(ctx)
		if err != nil {
			sentry.CaptureMessage(err.Error())
			sentry.Flush(time.Second * 2)
		}

		if !attempted && attemptedCount < 5 {
			// if it wasn't attempted try again up to max tries then bail
			attemptedCount++
			continue
		} else {
			// it was attempted, wait the specified duration
			attemptedCount = 0
			<-time.After(duration)
		}
	}
}

func main() {
	serverCtx, logger := setupLogger(context.Background())
	subLog := logger.Info().Str("prefix", "main")
	subLog.Msg("Starting server")

	serverCtx, r, _, jobs := setupRouter(serverCtx, logger)

	serverCtx, cancel := context.WithCancel(serverCtx)
	defer cancel()

	for _, job := range jobs {
		// iterate over jobs
		for i := 0; i < job.Workers; i++ {
			// spin up a job worker for each worker
			go jobWorker(serverCtx, job.Func, job.Cadence)
		}
	}

	srv := http.Server{Addr: ":3333", Handler: chi.ServerBaseContext(serverCtx, r)}
	err := srv.ListenAndServe()
	if err != nil {
		sentry.CaptureMessage(err.Error())
		sentry.Flush(time.Second * 2)
		logger.Panic().Err(err).Msg("HTTP server start failed!")
	}
}
