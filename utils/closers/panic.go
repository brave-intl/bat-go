package closers

import (
	"context"
	"io"

	loggingutils "github.com/brave-intl/bat-go/utils/logging"
)

// Panic calls Close on the specified closer, panicing on error
func Panic(c io.Closer) {
	_, logger := loggingutils.SetupLogger(context.Background())
	if c == nil {
		return
	}
	if err := c.Close(); err != nil {
		logger.Error().Err(err).Msg("error attempting to close")
		panic(err.Error())
	}
}
