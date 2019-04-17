package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/controllers"
	"github.com/brave-intl/bat-go/grant"
	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/promotion"
	"github.com/brave-intl/bat-go/utils/reputation"
	raven "github.com/getsentry/raven-go"
	"github.com/go-chi/chi"
	chiware "github.com/go-chi/chi/middleware"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
	"github.com/rs/zerolog/log"
)


func setupLogger(ctx context.Context) *zerolog.Logger {
	// set time field to unix
	zerolog.TimeFieldFormat = ""
	// always print out timestamp
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})
	return log.Logger.WithContext(ctx), &log.Logger
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
	r.Use(hlog.NewHandler(*logger))
	if logger != nil {
		// Also handles panic recovery
		r.Use(middleware.RequestLogger(logger))
	}

	grantPg, err := grant.NewPostgres("", true)
	if err != nil {
		raven.CaptureErrorAndWait(err, nil)
		log.Panic(err)
	}

	grantService, err := grant.InitService(grantPg)
	if err != nil {
		raven.CaptureErrorAndWait(err, nil)
		log.Panic(err)
	}

	pg, err := promotion.NewPostgres("", true)
	if err != nil {
		raven.CaptureErrorAndWait(err, nil)
		log.Panic().Err(err)
	}

	promotionService, err := promotion.InitService(pg)
	if err != nil {
		raven.CaptureErrorAndWait(err, nil)
		log.Panic(err)
	}

	r.Mount("/v1/grants", controllers.GrantsRouter(grantService))
	r.Mount("/v1/promotions", promotion.Router(promotionService))
	r.Mount("/v1/suggestions", promotion.SuggestionsRouter(promotionService))
	r.Get("/metrics", middleware.Metrics())

	env := os.Getenv("ENV")
	reputationServer := os.Getenv("REPUTATION_SERVER")
	reputationToken := os.Getenv("REPUTATION_TOKEN")
	if len(reputationServer) == 0 {
		if env == "production" {
			log.Panic().Err(errors.New("REPUTATION_SERVER is missing in production environment"))
		}
	} else {
		proxyRouter := reputation.ProxyRouter(reputationServer, reputationToken)
		r.Mount("/v1/devicecheck", proxyRouter)
		r.Mount("/v1/captchas", proxyRouter)
		r.Mount("/v2/attestations/safetynet", proxyRouter)
	}

	return ctx, r, promotionService
}

func main() {
	serverCtx, _ := setupLogger(context.Background())
	logger := log.Ctx(serverCtx)
	subLog := logger.Info().Str("prefix", "main")
	subLog.Msg("Starting server")

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		for {
			attempted, err := service.RunNextClaimJob(serverCtx)
			if err != nil {
				raven.CaptureErrorAndWait(err, nil)
			}
			if !attempted || err != nil {
				<-ticker.C
			}
		}
	}()

	srv := http.Server{Addr: ":3333", Handler: chi.ServerBaseContext(serverCtx, r)}
	err := srv.ListenAndServe()
	if err != nil {
		raven.CaptureErrorAndWait(err, nil)
		logger.Panic().Err(err)
	}
}
