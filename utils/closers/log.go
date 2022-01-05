package closers

import (
	"context"
	"io"

	loggingutils "github.com/brave-intl/bat-go/utils/logging"
)

// Log calls Close on the specified closer, panicing on error
func Log(ctx context.Context, c io.Closer) {
	_, logger := loggingutils.SetupLogger(ctx)

	if err := c.Close(); err != nil {
		logger.Error().Err(err).Msg("error attempting to close")
	}
}
