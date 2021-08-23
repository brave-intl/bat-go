package skuscli

import (
	"net/http"
	"os"
	"time"

	// pprof imports
	_ "net/http/pprof"

	"github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/skus"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/wallet"
	sentry "github.com/getsentry/sentry-go"
	"github.com/go-chi/chi"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// SkusRestRun - Main entrypoint of the REST subcommand
// This function takes a cobra command and starts up the
// skus rest microservice.
func SkusRestRun(command *cobra.Command, args []string) {

	ctx := command.Context()

	logger, err := appctx.GetLogger(ctx)
	cmd.Must(err)

	// setup generic middlewares and routes for health-check and metrics
	r := cmd.SetupRouter(ctx)

	var walletService *wallet.Service
	r, ctx, walletService = wallet.SetupService(ctx, r)

	skusPG, err := skus.NewPostgres("", true, "skus_db")
	if err != nil {
		sentry.CaptureException(err)
		logger.Panic().Err(err).Msg("Must be able to init postgres connection to start")
	}

	skusService, err := skus.InitService(ctx, skusPG, walletService)
	cmd.Must(err)

	r.Mount("/v1/credentials", skus.CredentialRouter(skusService))
	r.Mount("/v1/orders", skus.Router(skusService))
	// for skus webhook integrations
	r.Mount("/v1/webhooks", skus.WebhookRouter(skusService))
	r.Mount("/v1/votes", skus.VoteRouter(skusService))

	if os.Getenv("FEATURE_MERCHANT") != "" {
		skus.InitEncryptionKeys()
		skusDB, err := skus.NewPostgres("", true, "merch_skus_db")
		if err != nil {
			sentry.CaptureException(err)
			logger.Panic().Err(err).Msg("Must be able to init postgres connection to start")
		}
		skusService, err := skus.InitService(ctx, skusDB, walletService)
		if err != nil {
			sentry.CaptureException(err)
			logger.Panic().Err(err).Msg("Skus service initialization failed")
		}
		r.Mount("/v1/merchants", skus.MerchantRouter(skusService))
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
