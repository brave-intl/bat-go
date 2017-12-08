package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/controllers"
	"github.com/brave-intl/bat-go/datastore"
	"github.com/brave-intl/bat-go/grant"
	"github.com/brave-intl/bat-go/middleware"
	"github.com/garyburd/redigo/redis"
	"github.com/getsentry/raven-go"
	"github.com/go-chi/chi"
	chiware "github.com/go-chi/chi/middleware"
	"github.com/pressly/lg"
	"github.com/sirupsen/logrus"
)

var (
	redisURL = os.Getenv("REDIS_URL")
)

func main() {
	logger := logrus.New()

	//logger.Formatter = &logrus.JSONFormatter{}

	// Redirect output from the standard logging package "log"
	lg.RedirectStdlogOutput(logger)
	lg.DefaultLogger = logger
	serverCtx := context.Background()
	serverCtx = lg.WithLoggerContext(serverCtx, logger)

	logger.WithFields(logrus.Fields{"prefix": "main"}).Info("Starting server")

	govalidator.SetFieldsRequiredByDefault(true)

	r := chi.NewRouter()
	r.Use(chiware.RequestID)
	r.Use(chiware.RealIP)
	r.Use(chiware.Heartbeat("/"))
	r.Use(chiware.Timeout(60 * time.Second))
	r.Use(middleware.BearerToken)
	// Also handles panic recovery
	r.Use(middleware.RequestLogger(logger))

	redisAddress := "localhost:6379"
	if len(redisURL) > 0 {
		redisAddress = strings.TrimPrefix(redisURL, "redis://")
	}
	rp := &redis.Pool{
		MaxIdle:     3,
		IdleTimeout: 240 * time.Second,
		Dial:        func() (redis.Conn, error) { return redis.Dial("tcp", redisAddress) },
	}
	serverCtx = datastore.WithRedisPool(serverCtx, rp)

	err := grant.InitGrantService()
	if err != nil {
		raven.CaptureErrorAndWait(err, nil)
		log.Panic(err)
	}

	r.Mount("/v1/grants", controllers.GrantsRouter())
	r.Get("/metrics", middleware.Metrics())

	srv := http.Server{Addr: ":3333", Handler: chi.ServerBaseContext(serverCtx, r)}
	err = srv.ListenAndServe()
	if err != nil {
		raven.CaptureErrorAndWait(err, nil)
		log.Panic(err)
	}
}
