package payment

import (
	"context"

	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/rs/zerolog"
)

// logger - get a logger a little easier for payments
func logger(ctx context.Context) *zerolog.Logger {
	l, err := appctx.GetLogger(ctx)
	if err != nil {
		// create a new logger
		_, l = logging.SetupLogger(ctx)
	}
	sl := l.With().Str("module", "payments").Logger()
	return &sl
}
