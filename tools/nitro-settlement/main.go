package main

import (
	"context"

	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/tools/nitro-settlement/cmd"
)

func main() {
	// setup base context with logger
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// setup logger, and add to context
	ctx, logger := logging.SetupLogger(ctx)
	logger.Debug().Msg("running settlement-cli...")
	cmd.Execute(ctx)
}
