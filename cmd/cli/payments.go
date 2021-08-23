package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/brave-intl/bat-go/payments/pb"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
)

func prepareCmd(ctx context.Context) *cobra.Command {
	return &cobra.Command{
		Use:   "prepare",
		Short: "provides prepare access to payments micro-service entrypoint",
		Run: func(cmd *cobra.Command, args []string) {
			prepare(ctx, cmd, args)
		},
	}
}

func authorizeCmd(ctx context.Context) *cobra.Command {
	return &cobra.Command{
		Use:   "authorize",
		Short: "provides authorize access to payments micro-service entrypoint",
		Run: func(cmd *cobra.Command, args []string) {
			authorize(ctx, cmd, args)
		},
	}
}

func submitCmd(ctx context.Context) *cobra.Command {
	return &cobra.Command{
		Use:   "submit",
		Short: "provides submit access to payments micro-service entrypoint",
		Run: func(cmd *cobra.Command, args []string) {
			submit(ctx, cmd, args)
		},
	}
}

func grpcConnect(ctx context.Context) (grpc.ClientConnInterface, error) {
	// get the server address
	addr, ok := ctx.Value(appctx.PaymentsServiceCTXKey).(string)

	logger := logging.Logger(ctx, "grpcConnect").With().
		Str("payments-service", addr).Logger()

	if !ok || addr == "" {
		logger.Error().Msg("failed to get the payments service address")
		return nil, errors.New("failed to get the payments service address")
	}

	// dial
	var opts []grpc.DialOption
	conn, err := grpc.Dial(addr, opts...)
	if err != nil {
		logger.Error().Err(err).Msg("failed to dial payments service address")
		return nil, fmt.Errorf("failed to dial payments service address: %w", err)
	}

	return conn, nil
}

func prepare(ctx context.Context, command *cobra.Command, args []string) {
	// get the server address
	addr := ctx.Value(appctx.PaymentsServiceCTXKey).(string)
	// setup logger
	logger := logging.Logger(ctx, "prepare").With().
		Str("payments-service", addr).Logger()

	// connect
	logger.Info().Msg("connecting to payments service")
	conn, err := grpcConnect(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("failed to make connection to payments")
		return
	}

	// create the client
	client := pb.NewPaymentsGRPCServiceClient(conn)

	// perform api call
	// TODO: fill this out from input
	resp, err := client.Prepare(ctx, &pb.PrepareRequest{})
	if err != nil {
		logger.Error().Err(err).Msg("failed to make connection to payments")
		return
	}

	// note response
	logger.Info().
		Str("doc_id", resp.GetDocumentId()).
		Msg("prepare to payments service successful")
}

func authorize(ctx context.Context, command *cobra.Command, args []string) {
	// get the server address
	addr := ctx.Value(appctx.PaymentsServiceCTXKey).(string)
	// setup logger
	logger := logging.Logger(ctx, "authorize").With().
		Str("payments-service", addr).Logger()

	// connect
	logger.Info().Msg("connecting to payments service")
	conn, err := grpcConnect(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("failed to make connection to payments")
		return
	}

	// create the client
	client := pb.NewPaymentsGRPCServiceClient(conn)

	// perform api call
	// TODO: fill this out from input
	resp, err := client.Authorize(ctx, &pb.AuthorizeRequest{})
	if err != nil {
		logger.Error().Err(err).Msg("failed to make connection to payments")
		return
	}

	// note response
	logger.Info().
		Str("status", resp.GetMeta().GetStatus().String()).
		Msg("authorize to payments service successful")
}

func submit(ctx context.Context, command *cobra.Command, args []string) {
	// get the server address
	addr := ctx.Value(appctx.PaymentsServiceCTXKey).(string)
	// setup logger
	logger := logging.Logger(ctx, "submit").With().
		Str("payments-service", addr).Logger()

	// connect
	logger.Info().Msg("connecting to payments service")
	conn, err := grpcConnect(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("failed to make connection to payments")
		return
	}

	// create the client
	client := pb.NewPaymentsGRPCServiceClient(conn)

	// perform api call
	// TODO: fill this out from input
	resp, err := client.Authorize(ctx, &pb.AuthorizeRequest{})
	if err != nil {
		logger.Error().Err(err).Msg("failed to make connection to payments")
		return
	}

	// note response
	logger.Info().
		Str("status", resp.GetMeta().GetStatus().String()).
		Msg("submit to payments service successful")
}
