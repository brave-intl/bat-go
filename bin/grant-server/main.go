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
	raven "github.com/getsentry/raven-go"
	"github.com/go-chi/chi"
	chiware "github.com/go-chi/chi/middleware"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
	"github.com/rs/zerolog/log"
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

func setupRouter(ctx context.Context, logger *zerolog.Logger) (context.Context, *chi.Mux, *promotion.Service) {
	govalidator.SetFieldsRequiredByDefault(true)

	r := chi.NewRouter()
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
	if logger != nil {
		// Also handles panic recovery
		r.Use(hlog.NewHandler(*logger))
		r.Use(hlog.UserAgentHandler("user_agent"))
		r.Use(hlog.RequestIDHandler("req_id", "Request-Id"))
		r.Use(middleware.RequestLogger(logger))
	}

	roDB := os.Getenv("RO_DATABASE_URL")

	var grantRoPg grant.ReadOnlyDatastore
	grantPg, err := grant.NewPostgres("", true)
	if err != nil {
		raven.CaptureErrorAndWait(err, nil)
		log.Panic().Err(err).Msg("Must be able to init postgres connection to start")
	}
	if len(roDB) > 0 {
		grantRoPg, err = grant.NewPostgres(roDB, false)
		if err != nil {
			raven.CaptureErrorAndWait(err, nil)
			log.Error().Err(err).Msg("Could not start reader postgres connection")
		}
	}

	grantService, err := grant.InitService(grantPg, grantRoPg)
	if err != nil {
		raven.CaptureErrorAndWait(err, nil)
		log.Panic().Err(err).Msg("Grant service initialization failed")
	}

	var roPg promotion.ReadOnlyDatastore
	pg, err := promotion.NewPostgres("", true)
	if err != nil {
		raven.CaptureErrorAndWait(err, nil)
		log.Panic().Err(err).Msg("Must be able to init postgres connection to start")
	}
	if len(roDB) > 0 {
		roPg, err = promotion.NewPostgres(roDB, false)
		if err != nil {
			raven.CaptureErrorAndWait(err, nil)
			log.Error().Err(err).Msg("Could not start reader postgres connection")
		}
	}

	promotionService, err := promotion.InitService(pg, roPg)
	if err != nil {
		raven.CaptureErrorAndWait(err, nil)
		log.Panic().Err(err).Msg("Promotion service initialization failed")
	}

	r.Mount("/v1/grants", controllers.GrantsRouter(grantService))
	r.Mount("/v1/promotions", promotion.Router(promotionService))
	r.Mount("/v1/suggestions", promotion.SuggestionsRouter(promotionService))

	if os.Getenv("FEATURE_ORDERS") != "" {
		paymentPG, err := payment.NewPostgres("", true)
		if err != nil {
			raven.CaptureErrorAndWait(err, nil)
			log.Panic().Err(err).Msg("Must be able to init postgres connection to start")
		}
		paymentService, err := payment.InitService(paymentPG)
		if err != nil {
			raven.CaptureErrorAndWait(err, nil)
			log.Panic().Err(err).Msg("Payment service initialization failed")
		}
		r.Mount("/v1/orders", payment.Router(paymentService))
	}
	r.Get("/metrics", middleware.Metrics())

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

	return ctx, r, promotionService
}

func jobWorker(context context.Context, job func(context.Context) (bool, error), duration time.Duration) {
	ticker := time.NewTicker(duration)
	for {
		attempted, err := job(context)
		if err != nil {
			raven.CaptureErrorAndWait(err, nil)
		}
		if !attempted || err != nil {
			<-ticker.C
		}
	}
}

func main() {
	serverCtx, logger := setupLogger(context.Background())
	subLog := logger.Info().Str("prefix", "main")
	subLog.Msg("Starting server")

	serverCtx, r, service := setupRouter(serverCtx, logger)

	go jobWorker(serverCtx, service.RunNextClaimJob, 5*time.Second)
	go jobWorker(serverCtx, service.RunNextSuggestionJob, 5*time.Second)

	srv := http.Server{Addr: ":3333", Handler: chi.ServerBaseContext(serverCtx, r)}
	err := srv.ListenAndServe()
	if err != nil {
		raven.CaptureErrorAndWait(err, nil)
		logger.Panic().Err(err).Msg("HTTP server start failed!")
	}
}
