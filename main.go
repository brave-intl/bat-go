package main

import (
	"context"
	"net/http"
	"time"

	"github.com/brave-intl/bat-go/controllers"
	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils"
	"github.com/garyburd/redigo/redis"
	"github.com/go-chi/chi"
	chiware "github.com/go-chi/chi/middleware"
	"github.com/pressly/lg"
	"github.com/sirupsen/logrus"
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

	utils.InitValidators()

	r := chi.NewRouter()
	r.Use(chiware.RequestID)
	r.Use(chiware.RealIP)
	r.Use(chiware.Heartbeat("/"))
	r.Use(chiware.Timeout(60 * time.Second))
	r.Use(middleware.BearerToken)
	r.Use(middleware.Instrument)
	// Also handles panic recovery
	r.Use(middleware.RequestLogger(logger))

	rp := &redis.Pool{
		MaxIdle:     3,
		IdleTimeout: 240 * time.Second,
		Dial:        func() (redis.Conn, error) { return redis.Dial("tcp", "localhost:6379") },
	}
	serverCtx = utils.WithRedisPool(serverCtx, rp)

	r.Mount("/v1/grants", controllers.GrantsRouter())
	r.Get("/metrics", middleware.Metrics())

	srv := http.Server{Addr: ":3333", Handler: chi.ServerBaseContext(serverCtx, r)}
	srv.ListenAndServe()
}
