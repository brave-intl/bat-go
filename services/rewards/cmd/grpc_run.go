package cmd

import (

	// pprof imports
	_ "net/http/pprof"

	cmdutils "github.com/brave-intl/bat-go/cmd"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/spf13/cobra"
)

// GRPCRun - Main entrypoint of the GRPC subcommand
// This function takes a cobra command and starts up the
// rewards grpc microservice.
func GRPCRun(command *cobra.Command, args []string) {
	ctx := command.Context()
	logger, err := appctx.GetLogger(ctx)
	cmdutils.Must(err)
	// TODO: implement gRPC service
	logger.Fatal().Msg("gRPC server is not implemented")
}
