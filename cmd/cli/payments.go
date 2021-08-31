package cli

import (
	"context"
	"crypto/ed25519"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/brave-intl/bat-go/payments/pb"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
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
	// get the CA Cert for tls
	caCert := ctx.Value(appctx.CACertCTXKey).(string)

	logger := logging.Logger(ctx, "grpcConnect").With().
		Str("payments-service", addr).
		Str("ca-cert", caCert).
		Logger()

	if !ok || addr == "" {
		logger.Error().Msg("failed to get the payments service address")
		return nil, errors.New("failed to get the payments service address")
	}

	// dial
	var opts []grpc.DialOption

	if caCert != "" {
		creds, err := credentials.NewClientTLSFromFile(caCert, addr)
		if err != nil {
			logger.Error().Err(err).Msg("failed to create client tls")
			return nil, fmt.Errorf("failed to create client tls: %w", err)
		}
		opts = append(opts, grpc.WithTransportCredentials(creds), grpc.WithBlock())
	} else {
		opts = append(opts, grpc.WithInsecure(), grpc.WithBlock())
	}

	conn, err := grpc.Dial(addr, opts...)
	if err != nil {
		logger.Error().Err(err).Msg("failed to dial payments service address")
		return nil, fmt.Errorf("failed to dial payments service address: %w", err)
	}

	return conn, nil
}

func prepare(ctx context.Context, command *cobra.Command, args []string) {
	// setup logger
	logger := logging.Logger(ctx, "prepare")

	// connect
	logger.Info().Msg("connecting to payments service")
	conn, err := grpcConnect(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("failed to make connection to payments")
		return
	}

	// get the custodian value from input
	custodian, ok := ctx.Value(appctx.CustodianCTXKey).(string)
	if !ok {
		logger.Fatal().Msg("no custodian specified")
	}

	// get the payout file location
	f, ok := ctx.Value(appctx.PayoutFileLocationCTXKey).(string)
	if !ok {
		logger.Fatal().Msg("no payout file location specified")
	}

	// parse the payout report structure
	txs, err := parsePayoutFile(f)
	if err != nil {
		logger.Fatal().Err(err).Msg("unable to parse the payout file")
	}

	// create the client
	client := pb.NewPaymentsGRPCServiceClient(conn)

	// perform api call
	resp, err := client.Prepare(ctx, &pb.PrepareRequest{
		State:     pb.State_PREPARED,
		Custodian: pb.Custodian(pb.Custodian_value[strings.ToUpper(custodian)]),
		BatchTxs:  txs,
	})
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

	// get the key-pair file location
	f, ok := ctx.Value(appctx.KeyPairFileLocationCTXKey).(string)
	if !ok {
		logger.Fatal().Msg("no keypair file location specified for authorize")
	}

	// read der bytes from file
	b, err := ioutil.ReadFile(f)
	if err != nil {
		logger.Fatal().Err(err).Msg("unable to read key-pair file")
	}

	type ed25519PrivKey struct {
		Version          int
		ObjectIdentifier struct {
			ObjectIdentifier asn1.ObjectIdentifier
		}
		PrivateKey []byte
	}

	var block *pem.Block
	block, _ = pem.Decode(b)

	var asn1PrivKey ed25519PrivKey
	_, err = asn1.Unmarshal(block.Bytes, &asn1PrivKey)
	if err != nil {
		logger.Fatal().Err(err).Msg("unable to unmarshal private key")
	}

	privKey := ed25519.NewKeyFromSeed(asn1PrivKey.PrivateKey[2:])
	pubKey, ok := privKey.Public().(ed25519.PublicKey)
	if !ok {
		logger.Fatal().Msg("unable to get public key for specified private key")
	}

	// get the document id associated with batch
	docID, ok := ctx.Value(appctx.PaymentsDocumentIDCTXKey).(string)
	if !ok {
		logger.Fatal().Msg("no document id specified for authorize")
	}

	// perform api call
	resp, err := client.Authorize(ctx, &pb.AuthorizeRequest{
		DocumentId: docID,
		PublicKey:  hex.EncodeToString([]byte(pubKey)),
		Signature: base64.StdEncoding.EncodeToString(
			ed25519.Sign(privKey, []byte(docID))),
	})
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
