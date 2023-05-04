package cmd

import (
	"net/http"
	"time"

	// pprof imports
	_ "net/http/pprof"

	rootcmd "github.com/brave-intl/bat-go/cmd"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/middleware"
	"github.com/brave-intl/bat-go/services/cmd"
	"github.com/brave-intl/bat-go/services/payments"
	sentry "github.com/getsentry/sentry-go"
	"github.com/go-chi/chi"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// RestRun - Main entrypoint of the REST subcommand
// This function takes a cobra command and starts up the
// payments rest microservice.
func RestRun(command *cobra.Command, args []string) {
	ctx := command.Context()
	logger, err := appctx.GetLogger(ctx)
	rootcmd.Must(err)
	// add profiling flag to enable profiling routes
	if viper.GetString("pprof-enabled") != "" {
		// pprof attaches routes to default serve mux
		// host:6061/debug/pprof/
		go func() {
			logger.Error().Err(http.ListenAndServe(":6061", http.DefaultServeMux))
		}()
	}

	// setup the service now
	ctx, s, err := payments.NewService(ctx)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to initalize payments service")
	}

	// do rest endpoints
	r := cmd.SetupRouter(ctx)

	// prepare inserts transactions into qldb, returning a document which needs to be submitted by an authorizer
	r.Post("/v1/payments/prepare", middleware.InstrumentHandler("PrepareHandler", payments.PrepareHandler(s)).ServeHTTP)
	// submit will have an http signature from a known list of public keys
	r.Post("/v1/payments/submit", middleware.InstrumentHandler("SubmitHandler", s.AuthorizerSignedMiddleware()(payments.SubmitHandler(s))).ServeHTTP)
	// status to get the status and submission results from the submit
	r.Post("/v1/payments/{documentID}/status", middleware.InstrumentHandler("StatusHandler", payments.SubmitHandler(s)).ServeHTTP)

	// make sure exceptions go to sentry
	defer sentry.Flush(time.Second * 2)

	go func() {
		err := http.ListenAndServe(":9090", middleware.Metrics())
		if err != nil {
			sentry.CaptureException(err)
			logger.Panic().Err(err).Msg("metrics HTTP server start failed!")
		}
	}()

	// setup server, and run
	srv := http.Server{
		Addr:         viper.GetString("address"),
		Handler:      chi.ServerBaseContext(ctx, r),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 20 * time.Second,
	}

	if err = srv.ListenAndServe(); err != nil {
		sentry.CaptureException(err)
		logger.Fatal().Err(err).Msg("HTTP server start failed!")
	}
}
