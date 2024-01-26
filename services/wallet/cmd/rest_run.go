package cmd

import (
	"net/http"
	// pprof imports
	_ "net/http/pprof"
	"os"
	"strings"
	"time"

	cmdutils "github.com/brave-intl/bat-go/cmd"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/middleware"
	"github.com/brave-intl/bat-go/services/cmd"
	"github.com/brave-intl/bat-go/services/wallet"
	"github.com/getsentry/sentry-go"
	"github.com/go-chi/chi"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// WalletRestRun - Main entrypoint of the REST subcommand
// This function takes a cobra command and starts up the
// wallets rest microservice.
func WalletRestRun(command *cobra.Command, args []string) {
	ctx, service := wallet.SetupService(command.Context())

	logger, err := appctx.GetLogger(ctx)
	cmdutils.Must(err)

	router := cmd.SetupRouter(ctx)

	dappAO := strings.Split(os.Getenv("DAPP_ALLOWED_CORS_ORIGINS"), ",")
	if len(dappAO) == 0 {
		logger.Panic().Msg("dapp origin env missing")
	}

	wallet.RegisterRoutes(ctx, service, router, middleware.InstrumentHandler, wallet.NewDAppCorsMw(dappAO))

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
		Handler:      chi.ServerBaseContext(ctx, router),
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
