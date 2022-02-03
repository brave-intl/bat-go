package swaps

import (
	"fmt"
	"github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/swaps"
	appctx "github.com/brave-intl/bat-go/utils/context"
	sentry "github.com/getsentry/sentry-go"
	"github.com/spf13/cobra"
	"time"
)

// Run - Main entrypoint for the swaps
// This function takes a cobra command and starts up the swap rewards service
func Run(command *cobra.Command, args []string) {
	ctx := command.Context()
	logger, err := appctx.GetLogger(ctx)
	cmd.Must(err)

	// add our command line params to context
	// TODO

	// setup the service now
	ctx, service, err := swaps.InitService(ctx)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to initalize swaps service")
	}

	// do rest endpoints
	// r := cmd.SetupRouter(command.Context())
	// r.Get("/v1/parameters", middleware.InstrumentHandler(
	// 	"GetParametersHandler", rewards.GetParametersHandler(s)).ServeHTTP)

	// make sure exceptions go to sentry
	defer sentry.Flush(time.Second * 2)

	// go func() {
	// 	err := http.ListenAndServe(":9090", middleware.Metrics())
	// 	if err != nil {
	// 		sentry.CaptureException(err)
	// 		logger.Panic().Err(err).Msg("metrics HTTP server start failed!")
	// 	}
	// }()

	// setup server, and run
	// srv := http.Server{
	// 	Addr:         viper.GetString("address"),
	// 	Handler:      chi.ServerBaseContext(ctx, r),
	// 	ReadTimeout:  5 * time.Second,
	// 	WriteTimeout: 20 * time.Second,
	// }

	// if err = srv.ListenAndServe(); err != nil {
	// 	sentry.CaptureException(err)
	// 	logger.Fatal().Err(err).Msg("HTTP server start failed!")
	// }

	fmt.Println(service)
}
