package cmd

import (
	"context"
	"net/http"
	"time"

	// pprof imports
	_ "net/http/pprof"

	"github.com/brave-intl/bat-go/middleware"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/getsentry/sentry-go"
	"github.com/go-chi/chi"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// SetupWalletService - setup the wallet microservice
func SetupWalletService(ctx context.Context, r *chi.Mux) (*chi.Mux, context.Context, *wallet.Service) {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		// no logger, setup
		ctx, logger = logging.SetupLogger(ctx)
	}

	// setup the service now
	db, err := wallet.NewWritablePostgres(viper.GetString("datastore"), false, "wallet_db")
	if err != nil {
		logger.Panic().Err(err).Msg("unable connect to wallet db")
	}
	roDB, err := wallet.NewReadOnlyPostgres(viper.GetString("ro-datastore"), false, "wallet_ro_db")
	if err != nil {
		logger.Panic().Err(err).Msg("unable connect to wallet db")
	}

	ctx = context.WithValue(ctx, appctx.RODatastoreCTXKey, roDB)
	ctx = context.WithValue(ctx, appctx.DatastoreCTXKey, db)

	// add our command line params to context
	ctx = context.WithValue(ctx, appctx.EnvironmentCTXKey, viper.Get("environment"))

	s, err := wallet.InitService(ctx, db, roDB)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to initialize wallet service")
	}

	// if feature is enabled, setup the routes
	if viper.GetBool("wallets-feature-flag") {
		// setup our wallet routes
		r.Route("/v3/wallet", func(r chi.Router) {
			// create wallet routes for our wallet providers
			r.Post("/uphold", middleware.InstrumentHandlerFunc(
				"CreateUpholdWallet", wallet.CreateUpholdWalletV3))
			r.Post("/brave", middleware.InstrumentHandlerFunc(
				"CreateBraveWallet", wallet.CreateBraveWalletV3))

			// create wallet claim routes for our wallet providers
			r.Post("/uphold/{paymentID}/claim", middleware.InstrumentHandlerFunc(
				"ClaimUpholdWallet", wallet.ClaimUpholdWalletV3(s)))

			// get wallet routes
			r.Get("/{paymentID}", middleware.InstrumentHandlerFunc(
				"GetWallet", wallet.GetWalletV3))
			r.Get("/recover/{publicKey}", middleware.InstrumentHandlerFunc(
				"RecoverWallet", wallet.RecoverWalletV3))

			// get wallet balance routes
			r.Get("/uphold/{providerID}", middleware.InstrumentHandlerFunc(
				"GetUpholdWalletBalance", wallet.GetUpholdWalletBalanceV3))
		})
	}
	return r, ctx, s
}

// WalletRestRun - Main entrypoint of the REST subcommand
// This function takes a cobra command and starts up the
// wallets rest microservice.
func WalletRestRun(cmd *cobra.Command, args []string) {
	// setup generic middlewares and routes for health-check and metrics
	r := setupRouter(ctx)
	r, ctx, _ = SetupWalletService(ctx, r)

	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		// no logger, setup
		ctx, logger = logging.SetupLogger(ctx)
	}

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
