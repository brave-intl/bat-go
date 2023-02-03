package internal

import (
	"context"
	"fmt"

	"github.com/brave-intl/bat-go/libs/logging"
)

// LogAndError - helper to log and error
func LogAndError(ctx context.Context, err error, prefix, msg string) error {
	if err == nil {
		err = fmt.Errorf("%s", msg)
	}
	logging.Logger(ctx, prefix).Error().Err(err).Msg(msg)
	return fmt.Errorf("%s: %w", msg, err)
}
