package cmd

import (
	"net/http"
	"time"

	// pprof imports
	_ "net/http/pprof"

	"github.com/brave-intl/bat-go/services/cmd"
	"github.com/brave-intl/bat-go/services/wallet"
	cmdutils "github.com/brave-intl/bat-go/libs/cmd"
	appctx "github.com/brave-intl/bat-go/libs/context"
	sentry "github.com/getsentry/sentry-go"
	"github.com/go-chi/chi"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// WalletRestRun - Main entrypoint of the REST subcommand
// This function takes a cobra command and starts up the
// wallets rest microservice.
func WalletRestRun(command *cobra.Command, args []string) {
	// setup generic middlewares and routes for health-check and metrics
	r := cmd.SetupRouter(command.Context())
	r, ctx, _ := wallet.SetupService(command.Context(), r)
	logger, err := appctx.GetLogger(ctx)
	cmdutils.Must(err)

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
