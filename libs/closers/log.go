package closers

import (
	"context"
	"io"

	loggingutils "github.com/brave-intl/bat-go/libs/logging"
)

// Log calls Close on the specified closer, panicing on error
func Log(ctx context.Context, c io.Closer) {
	// get the logger from the context
	logger := loggingutils.Logger(ctx, "closer.Log")

	if err := c.Close(); err != nil {
		logger.Error().Err(err).Msg("error attempting to close")
	}
}
