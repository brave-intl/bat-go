package closers

import (
	"context"
	"errors"
	"io"

	"github.com/brave-intl/bat-go/libs/logging"
)

// Panic calls Close on the specified closer, panicking on error
func Panic(ctx context.Context, c io.Closer) {
	logger := logging.Logger(ctx, "closers.Panic")
	if c == nil {
		return
	}
	if err := c.Close(); err != nil {
		logger.Error().Err(err).Msg("error attempting to close")
		if errors.Is(err, context.Canceled) || err.Error() == "context canceled" {
			// after this is merged we can remove this, the context timeout
			// on the http client will manifest into this if the stream is not
			// completed in time as "impact from not canceling the context is minor"
			// https://go-review.googlesource.com/c/go/+/361919/
			// TODO: remove this when ^^ is released
			return
		}
		panic(err.Error())
	}
}
