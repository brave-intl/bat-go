package payments

import (

	// pprof imports
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"

	"github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/payment"
	appctx "github.com/brave-intl/bat-go/utils/context"
	sentry "github.com/getsentry/sentry-go"
	"github.com/go-chi/chi"
	"github.com/go-chi/cors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// PaymentRestRun - Main entrypoint of the REST subcommand
// This function takes a cobra command and starts up the
// payments rest microservice.
func PaymentRestRun(command *cobra.Command, args []string) {
	// setup generic middlewares and routes for health-check and metrics
	r := cmd.SetupRouter(command.Context())

	if os.Getenv("ENV") != "production" {
		r.Use(cors.Handler(cors.Options{
			Debug:            true,
			AllowedOrigins:   []string{"https://confab.bsg.brave.software", "https://together.bsg.brave.software"},
			AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "Digest", "Signature"},
			ExposedHeaders:   []string{"Link"},
			AllowCredentials: false,
			MaxAge:           300,
		}))
	}

	r = cmd.SetupDefaultRoutes(command.Context(), r)
	r, ctx, _ := payment.SetupService(command.Context(), r)
	logger, err := appctx.GetLogger(ctx)

	cmd.Must(err)

	// add profiling flag to enable profiling routes
	logger.Info().Msg("setting up pprof for service, port 6061")
	if viper.GetString("pprof-enabled") != "" {
		// pprof attaches routes to default serve mux
		// host:6061/debug/pprof/
		go func() {
			logger.Error().Err(http.ListenAndServe(":6061", http.DefaultServeMux))
		}()
	}

	logger.Info().Msg("creating web server")
	// setup server, and run
	srv := http.Server{
		Addr:         viper.GetString("address"),
		Handler:      chi.ServerBaseContext(ctx, r),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 20 * time.Second,
	}

	// make sure exceptions go to sentry
	defer sentry.Flush(time.Second * 2)

	logger.Info().Msg("server listening")
	if err = srv.ListenAndServe(); err != nil {
		sentry.CaptureException(err)
		logger.Fatal().Err(err).Msg("HTTP server start failed!")
	}
	<-time.After(2 * time.Second)
}
