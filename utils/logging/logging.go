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

// SetupLoggerWithLevel - helper to setup a logger and associate with context with a given log level
func SetupLoggerWithLevel(ctx context.Context, level zerolog.Level) (context.Context, *zerolog.Logger) {
	// setup context with log level passed in
	ctx = context.WithValue(ctx, appctx.LogLevelCTXKey, level)
	// call SetupLogger
	return SetupLogger(ctx)
}

// SetupLogger - helper to setup a logger and associate with context
func SetupLogger(ctx context.Context) (context.Context, *zerolog.Logger) {
	env, err := appctx.GetStringFromContext(ctx, appctx.EnvironmentCTXKey)
	if err != nil {
		// if not in context, default to local
		env = "local"
	}

	// defaults to info level
	level, _ := appctx.GetLogLevelFromContext(ctx, appctx.LogLevelCTXKey)

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

	// set the log level
	l = l.Level(level)

	// debug override
	if debug {
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

// Progress - type to store the incremental progress of a task
type Progress struct {
	Processed int
	Total     int
}

// SubmitProgress - helper to log progress
func SubmitProgress(ctx context.Context, processed, total int) {
	progChan, progOk := ctx.Value(appctx.ProgressLoggingCTXKey).(chan Progress)
	if progOk {
		progChan <- Progress{
			Processed: processed,
			Total:     total,
		}
	}
}

// ReportProgress - goroutine watching for Progress updates for logging
func ReportProgress(ctx context.Context, progressDuration time.Duration) chan Progress {
	// setup logger
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		_, logger = SetupLogger(ctx)
	}

	// we will return the progress channel so the app
	// can send us progress information as it processes
	progChan := make(chan Progress)
	var (
		last Progress
	)
	go func() {
		for {
			select {
			case <-time.After(progressDuration):
				// output most resent progress information
				logger.Info().
					Int("processed", last.Processed).
					Int("pending", last.Total-last.Processed).
					Int("total", last.Total).
					Msg("progress update")
			case last = <-progChan:
				continue
			}
		}
	}()
	return progChan
}

// Logger - get a logger a little easier for payments
func Logger(ctx context.Context, prefix string) *zerolog.Logger {
	l, err := appctx.GetLogger(ctx)
	if err != nil {
		// create a new logger
		_, l = SetupLogger(ctx)
	}
	sl := l.With().Str("module", prefix).Logger()
	return &sl
}
