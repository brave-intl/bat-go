package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	appctx "github.com/brave-intl/bat-go/libs/context"
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
	Writer io.WriteCloser
)

func NopCloser(w io.Writer) io.WriteCloser {
	return nopCloser{w}
}

type nopCloser struct {
	io.Writer
}

func (nopCloser) Close() error { return nil }

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
	writer, ok := ctx.Value(appctx.LogWriterCTXKey).(io.Writer)

	env, err := appctx.GetStringFromContext(ctx, appctx.EnvironmentCTXKey)
	if err != nil {
		// if not in context, default to local
		env = "local"
	}

	// defaults to info level
	level, _ := appctx.GetLogLevelFromContext(ctx, appctx.LogLevelCTXKey)

	if ok {
		Writer = NopCloser(writer)
	} else if env != "local" {
		// this log writer uses a ring buffer and drops messages that cannot be processed
		// in a timely manner
		Writer = diode.NewWriter(os.Stdout, 1000, time.Duration(20*time.Millisecond), func(missed int) {
			// add to our counter of lost log messages
			droppedLogTotal.Add(float64(missed))
		})
	} else {
		Writer = NopCloser(zerolog.ConsoleWriter{Out: os.Stdout})
	}

	// always print out timestamp
	l := zerolog.New(Writer).With().Timestamp().Logger()

	var (
		debug bool
	)

	// set the log level
	l = l.Level(level)

	// debug override
	if debug, ok = ctx.Value(appctx.DebugLoggingCTXKey).(bool); ok && debug {
		l = l.Level(zerolog.DebugLevel)
	}

	return l.WithContext(ctx), &l
}

func UpdateContext(ctx context.Context, logger zerolog.Logger) (context.Context, *zerolog.Logger) {
	ctx = logger.WithContext(ctx)
	return ctx, &logger
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
				// output most recent progress information, but only if
				// some progress has been made.
				if last.Processed != 0 && last.Total-last.Processed != 0 && last.Total != 0 {
					logger.Info().
						Int("processed", last.Processed).
						Int("pending", last.Total-last.Processed).
						Int("total", last.Total).
						Msg("progress update")
				}
			case last = <-progChan:
				continue
			}
		}
	}()
	return progChan
}

// UpholdProgress - type to store the incremental progress of an Uphold transaction set
type UpholdProgress struct {
	Message string
	Count   int
}

// UpholdProgressSet - the set up uphold progresses
type UpholdProgressSet struct {
	Progress []UpholdProgress
}

// UpholdSubmitProgress - helper to log progress
func UpholdSubmitProgress(ctx context.Context, progressSet UpholdProgressSet) {
	progChan, progOk := ctx.Value(appctx.ProgressLoggingCTXKey).(chan UpholdProgressSet)
	if progOk {
		progChan <- progressSet
	}
}

// UpholdReportProgress - goroutine watching for UpholdProgress updates for logging
func UpholdReportProgress(ctx context.Context, progressDuration time.Duration) chan UpholdProgressSet {
	// setup logger
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		_, logger = SetupLogger(ctx)
	}

	// we will return the progress channel so the app
	// can send us progress information as it processes
	progChan := make(chan UpholdProgressSet)
	var (
		last UpholdProgressSet
	)
	go func() {
		for {
			select {
			case <-time.After(progressDuration):
				// output most recent progress information, but only if
				// some progress has been made.
				if len(last.Progress) > 0 {
					prettyProgress, err := json.MarshalIndent(last.Progress, "", "  ")
					if err == nil {
						logger.Info().Msg(fmt.Sprintf("progress update:\n%s", prettyProgress))
					} else {
						logger.Error().Err(err).Msg("failed to prettify progress for logging")
					}
				}
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

// FromContext - retrieves logger from context or gets a new logger if not present
func FromContext(ctx context.Context) *zerolog.Logger {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		_, logger = SetupLogger(ctx)
	}
	return logger
}

// LogAndError - helper to log and error
func LogAndError(logger *zerolog.Logger, msg string, err error) error {
	if logger != nil {
		logger.Error().Err(err).Msg(msg)
	}
	return err
}
