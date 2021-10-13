package payments

import (
	"context"
	"fmt"

	"github.com/brave-intl/bat-go/cmd"
	appctx "github.com/brave-intl/bat-go/utils/context"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
)

var (
	//PaymentsCmd -  top level payments subcommand
	PaymentsCmd = &cobra.Command{
		Use:   "payments",
		Short: "provides payout processing capabilities",
	}

	// LoadCmd - uploads a given payout report to the payments grpc service "prepare" handler
	LoadCmd = &cobra.Command{
		Use:   "load",
		Short: "load payout report to payments service",
		Run:   cmd.Perform("load", RunLoad),
	}

	// AuthorizeCmd - uploads a given payout report to the payments grpc service "prepare" handler
	AuthorizeCmd = &cobra.Command{
		Use:   "authorize",
		Short: "authorize payout report to payments service",
		Run:   cmd.Perform("authorize", RunAuthorize),
	}

	// SubmitCmd - uploads a given payout report to the payments grpc service "prepare" handler
	SubmitCmd = &cobra.Command{
		Use:   "submit",
		Short: "submit payout report to payments service",
		Run:   cmd.Perform("submit", RunSubmit),
	}
)

func init() {
	// add the root payments cli cmd
	cmd.RootCmd.AddCommand(PaymentsCmd)

	// LoadCmd - ../bat-go payments load ...
	PaymentsCmd.AddCommand(LoadCmd)
	// AuthorizeCmd - ../bat-go payments authorize ...
	PaymentsCmd.AddCommand(AuthorizeCmd)
	// SubmitCmd - ../bat-go payments submit ...
	PaymentsCmd.AddCommand(SubmitCmd)

	// flags for load subcommand
	loadBuilder := cmd.NewFlagBuilder(LoadCmd)

	loadBuilder.Flag().Bool("verbose", false,
		"how verbose logging should be").
		Bind("verbose")

	// flags for submit subcommand
	submitBuilder := cmd.NewFlagBuilder(SubmitCmd)

	submitBuilder.Flag().Bool("verbose", false,
		"how verbose logging should be").
		Bind("verbose")

	// flags for authorize subcommand
	authorizeBuilder := cmd.NewFlagBuilder(AuthorizeCmd)

	authorizeBuilder.Flag().Bool("verbose", false,
		"how verbose logging should be").
		Bind("verbose")

	authorizeBuilder.Flag().String("private-key-path", "",
		"path to authorizers' private key").
		Bind("private-key-path").
		Require()

	authorizeBuilder.Flag().String("public-key-id", "",
		"uuid of the public key corresponding to private key").
		Bind("private-key-path").
		Require()

}

// RunLoad - entrypoint for the load command
func RunLoad(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// do we need verbose logging
	verbose, err := cmd.Flags().GetBool("verbose")
	if err != nil {
		return err
	}
	// setup context for logging, debug and progress
	ctx = context.WithValue(ctx, appctx.DebugLoggingCTXKey, verbose)
	return Load(ctx, args...)
}

// RunAuthorize - entrypoint for the authorize command
func RunAuthorize(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	sublogger := logger(ctx).With().Str("func", "payments.Load").Logger()

	// do we need verbose logging
	verbose, err := cmd.Flags().GetBool("verbose")
	if err != nil {
		sublogger.Error().Err(err).Msg("failed to get verbosity level")
		return fmt.Errorf("failed to failed to get verbosity level: %w", err)
	}
	// setup context for logging, debug and progress
	ctx = context.WithValue(ctx, appctx.DebugLoggingCTXKey, verbose)

	docIDs := []*uuid.UUID{}
	for _, docIDStr := range args {
		docID, err := uuid.Parse(docIDStr)
		if err != nil {
			sublogger.Error().Err(err).Msg("failed to parse document id from command line.")
			return fmt.Errorf("failed to parse document id: %w", err)
		}

		docIDs = append(docIDs, &docID)
	}

	// get the private key path from command line option
	privKeyPath, err := cmd.Flags().GetString("private-key-path")
	if err != nil {
		sublogger.Error().Err(err).Msg("failed to get private key path")
		return fmt.Errorf("failed to failed to get private key path: %w", err)
	}

	// get the public key uuid from command line option
	pubKeyIDStr, err := cmd.Flags().GetString("public-key-id")
	if err != nil {
		sublogger.Error().Err(err).Msg("failed to get public keyid")
		return fmt.Errorf("failed to failed to get public key id: %w", err)
	}
	pubKeyID, err := uuid.Parse(pubKeyIDStr)
	if err != nil {
		sublogger.Error().Err(err).Msg("failed to parse public key id from command line.")
		return fmt.Errorf("failed to parse public key id: %w", err)
	}

	return Authorize(ctx, &pubKeyID, privKeyPath, docIDs...)
}

// RunSubmit - entrypoint for the authorize command
func RunSubmit(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	sublogger := logger(ctx).With().Str("func", "payments.Submit").Logger()

	// do we need verbose logging
	verbose, err := cmd.Flags().GetBool("verbose")
	if err != nil {
		sublogger.Error().Err(err).Msg("failed to get verbosity level")
		return fmt.Errorf("failed to failed to get verbosity level: %w", err)
	}
	// setup context for logging, debug and progress
	ctx = context.WithValue(ctx, appctx.DebugLoggingCTXKey, verbose)

	docIDs := []*uuid.UUID{}
	for _, docIDStr := range args {
		docID, err := uuid.Parse(docIDStr)
		if err != nil {
			sublogger.Error().Err(err).Msg("failed to parse document id from command line.")
			return fmt.Errorf("failed to parse document id: %w", err)
		}

		docIDs = append(docIDs, &docID)
	}
	return Submit(ctx, docIDs...)
}

// Load - performs the loading to `prepare` payments service endpoint
func Load(ctx context.Context, reportURIs ...string) error {
	sublogger := logger(ctx).With().Str("func", "payments.Load").Logger()
	sublogger.Warn().Msg("Load function is not yet implemented")
	return errorutils.ErrNotImplemented
}

// Authorize - performs the loading to `prepare` payments service endpoint
func Authorize(ctx context.Context, pubKeyID *uuid.UUID, privKeyPath string, docIDs ...*uuid.UUID) error {
	sublogger := logger(ctx).With().Str("func", "payments.Authorize").Logger()
	sublogger.Warn().Msg("Authorize function is not yet implemented")
	return errorutils.ErrNotImplemented
}

// Submit - performs the loading to `prepare` payments service endpoint
func Submit(ctx context.Context, docID ...*uuid.UUID) error {
	sublogger := logger(ctx).With().Str("func", "payments.Submit").Logger()
	sublogger.Warn().Msg("Submit function is not yet implemented")
	return errorutils.ErrNotImplemented
}

// logger - get the logger no matter what
func logger(ctx context.Context) *zerolog.Logger {
	// setup logger, with the context that has the logger
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		_, logger = logging.SetupLogger(ctx)
	}
	return logger
}
