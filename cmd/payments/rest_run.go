package payments

import (

	// pprof imports
	_ "net/http/pprof"

	"github.com/brave-intl/bat-go/cmd"
	appctx "github.com/brave-intl/bat-go/utils/context"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/spf13/cobra"
)

// RestRun - Main entrypoint of the REST subcommand
// This function takes a cobra command and starts up the
// rewards rest microservice.
func RestRun(command *cobra.Command, args []string) {
	logger, err := appctx.GetLogger(command.Context())
	cmd.Must(err)
	logger.Fatal().Err(errorutils.ErrNotImplemented).Msg("Rest Service Not Implemented")
}
