package payments

import (

	// pprof imports
	"net/http"
	_ "net/http/pprof"
	"time"

	"github.com/brave-intl/bat-go/cmd"
	"github.com/getsentry/sentry-go"
	"github.com/go-chi/chi"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// PaymentRestRun - Main entrypoint of the REST subcommand
// This function takes a cobra command and starts up the
// payments rest microservice.
func PaymentRestRun(command *cobra.Command, args []string) {
	// setup generic middlewares and routes for health-check and metrics
	r := cmd.SetupRouter(command.Context())
	r, ctx, _ := payment.SetupService(command.Context(), r)
	logger, err := appctx.GetLogger(ctx)
	cmd.Must(err)

	// add profiling flag to enable profiling routes
	if viper.GetString("pprof-enabled") != "" {
		// pprof attaches routes to default serve mux
		// host:6061/debug/pprof/
		go func() {
			logger.Error().Err(http.ListenAndServe(":6061", http.DefaultServeMux))
		}()
	}

	// setup server, and run
	srv := http.Server{
		Addr:         viper.GetString("address"),
		Handler:      chi.ServerBaseContext(ctx, r),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 20 * time.Second,
	}

	// make sure exceptions go to sentry
	defer sentry.Flush(time.Second * 2)

	if err = srv.ListenAndServe(); err != nil {
		sentry.CaptureException(err)
		logger.Fatal().Err(err).Msg("HTTP server start failed!")
	}
}
