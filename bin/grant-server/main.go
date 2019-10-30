package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/controllers"
	"github.com/brave-intl/bat-go/grant"
	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/promotion"
	"github.com/brave-intl/bat-go/utils/reputation"
	"github.com/garyburd/redigo/redis"
	raven "github.com/getsentry/raven-go"
	"github.com/go-chi/chi"
	chiware "github.com/go-chi/chi/middleware"
	"github.com/pressly/lg"
	"github.com/sirupsen/logrus"
)

var (
	redisURL = os.Getenv("REDIS_URL")
)

func setupLogger(ctx context.Context) (context.Context, *logrus.Logger) {
	logger := logrus.New()

	//logger.Formatter = &logrus.JSONFormatter{}

	// Redirect output from the standard logging package "log"
	lg.RedirectStdlogOutput(logger)
	lg.DefaultLogger = logger
	ctx = lg.WithLoggerContext(ctx, logger)
	return ctx, logger
}

func setupRouter(ctx context.Context, logger *logrus.Logger) (context.Context, *chi.Mux) {
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
	r.Use(middleware.RateLimiter())
	if logger != nil {
		// Also handles panic recovery
		r.Use(middleware.RequestLogger(logger))
	}

	redisAddress := "localhost:6379"
	if len(redisURL) > 0 {
		redisAddress = strings.TrimPrefix(redisURL, "redis://")
	}
	rp := &redis.Pool{
		MaxIdle:     3,
		IdleTimeout: 240 * time.Second,
		Dial:        func() (redis.Conn, error) { return redis.Dial("tcp", redisAddress) },
	}

	grantPg, err := grant.NewPostgres("", true)
	if err != nil {
		raven.CaptureErrorAndWait(err, nil)
		log.Panic(err)
	}

	grantService, err := grant.InitService(grantPg, rp)
	if err != nil {
		raven.CaptureErrorAndWait(err, nil)
		log.Panic(err)
	}

	pg, err := promotion.NewPostgres("", true)
	if err != nil {
		raven.CaptureErrorAndWait(err, nil)
		log.Panic(err)
	}

	promotionService, err := promotion.InitService(pg)
	if err != nil {
		raven.CaptureErrorAndWait(err, nil)
		log.Panic(err)
	}

	r.Mount("/v1/grants", controllers.GrantsRouter(grantService))
	r.Mount("/v1/promotions", promotion.Router(promotionService))
	//r.Mount("/v1/suggestions", promotion.SuggestionRouter(promotionService))
	r.Get("/metrics", middleware.Metrics())

	if len(os.Getenv("REPUTATION_SERVER")) == 0 {
		if os.Getenv("ENV") == "production" {
			log.Panic(errors.New("REPUTATION_SERVER is missing in production environment"))
		}
	} else {
		r.Mount("/v1/devicecheck", reputation.ProxyRouter())
	}

	return ctx, r
}

func main() {
	serverCtx, logger := setupLogger(context.Background())

	logger.WithFields(logrus.Fields{"prefix": "main"}).Info("Starting server")

	serverCtx, r := setupRouter(serverCtx, logger)

	srv := http.Server{Addr: ":3333", Handler: chi.ServerBaseContext(serverCtx, r)}
	err := srv.ListenAndServe()
	if err != nil {
		raven.CaptureErrorAndWait(err, nil)
		log.Panic(err)
	}
}
