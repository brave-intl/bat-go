package logging

import (
	"context"
	"io"
	"os"

	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/rs/zerolog"
	uuid "github.com/satori/go.uuid"
)

// SetupLogger - helper to setup a logger and associate with context
func SetupLogger(ctx context.Context) (context.Context, *zerolog.Logger) {
	var (
		env, err = appctx.GetStringFromContext(ctx, appctx.EnvironmentCTXKey)
	)
	if err != nil {
		// if not in context, default to local
		env = "local"
	}
	var output io.Writer
	if env != "local" {
		output = os.Stdout
	} else {
		output = zerolog.ConsoleWriter{Out: os.Stdout}
	}

	// always print out timestamp
	l := zerolog.New(output).With().Timestamp().Logger()

	debug := os.Getenv("DEBUG")
	if debug == "" || debug == "f" || debug == "n" || debug == "0" {
		l = l.Level(zerolog.InfoLevel)
	}

	return l.WithContext(ctx), &l
}

// AddWalletIDToContext adds wallet id to context
func AddWalletIDToContext(ctx context.Context, walletID uuid.UUID) {
	l := zerolog.Ctx(ctx)
	if e := l.Debug(); e.Enabled() {
		l.UpdateContext(func(c zerolog.Context) zerolog.Context {
			return c.Str("walletID", walletID.String())
		})
	}
}
