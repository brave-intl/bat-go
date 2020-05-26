package cmd

import (

	// pprof imports
	_ "net/http/pprof"

	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/spf13/cobra"
)

// RewardsGRPCRun - Main entrypoint of the GRPC subcommand
// This function takes a cobra command and starts up the
// rewards grpc microservice.
func RewardsGRPCRun(cmd *cobra.Command, args []string) {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		// no logger, setup
		ctx, logger = logging.SetupLogger(ctx)
	}
	// TODO: implement gRPC service
	logger.Fatal().Msg("gRPC server is not implemented")
}
