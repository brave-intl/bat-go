package logging

import (
	"context"
	"io"
	"os"
	"strconv"
	"time"

	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/diode"
	uuid "github.com/satori/go.uuid"
)

var (
	// we are not promising to get every log message in the log
	// anymore, when it comes down to it, we would rather the service
	// runs than fails on log writing contention.  This will let us
	// see how many logs we are dropping
	droppedLogTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "dropped_log_events_total",
			Help: "A counter for the number of dropped log messages",
		},
	)
)

func init() {
	prometheus.MustRegister(droppedLogTotal)
}

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
		// this log writer uses a ring buffer and drops messages that cannot be processed
		// in a timely manner
		output = diode.NewWriter(os.Stdout, 1000, time.Duration(20*time.Millisecond), func(missed int) {
			// add to our counter of lost log messages
			droppedLogTotal.Add(float64(missed))
		})
	} else {
		output = zerolog.ConsoleWriter{Out: os.Stdout}
	}

	// always print out timestamp
	l := zerolog.New(output).With().Timestamp().Logger()

	var (
		debug bool
		ok    bool
	)

	// use context to get debugging flag first, then fall back to env variable
	if debug, ok = ctx.Value("debug_logging").(bool); !ok {
		if os.Getenv("DEBUG") == "" {
			// false, but ParseBool doesn't understand empty string
			debug = false
		} else if debug, err = strconv.ParseBool(os.Getenv("DEBUG")); err != nil {
			// parse error
			debug = false
		}
	}

	if !debug {
		// if not debug, set log level to info
		l = l.Level(zerolog.InfoLevel)
	} else {
		l = l.Level(zerolog.DebugLevel)
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
