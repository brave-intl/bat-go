package payments

import (
	"net"
	"net/http"

	// pprof imports
	_ "net/http/pprof"

	"github.com/brave-intl/bat-go/payments"
	paymentsPB "github.com/brave-intl/bat-go/payments/pb"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
)

// GRPCRun - Main entrypoint of the GRPC subcommand
// This function takes a cobra command and starts up the
// rewards grpc microservice.
func GRPCRun(command *cobra.Command, args []string) {
	ctx := command.Context()

	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		panic("failed to setup logger for payments")
	}

	logger.Debug().Msg("setting up payments service")

	// setup pprof if enabled

	// add profiling flag to enable profiling routes
	if viper.GetString("pprof-enabled") != "" {
		// pprof attaches routes to default serve mux
		// host:6061/debug/pprof/
		go func() {
			logger.Info().Str("addr", ":6061").Msg("serving grpc service pprof port")
			logger.Error().Err(http.ListenAndServe(":6061", http.DefaultServeMux))
		}()
	}

	addr, ok := ctx.Value(appctx.SrvAddrCTXKey).(string)
	if !ok || addr == "" {
		logger.Fatal().Err(err).Msg("failed to get server address for payments")
	}

	logger.Debug().Msg("setting up listener for payments")
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to setup listener for payments")
	}

	pSrv := new(payments.Service)

	// setup grpc service
	var opts []grpc.ServerOption
	gSrv := grpc.NewServer(opts...)

	paymentsPB.RegisterPaymentsGRPCServiceServer(gSrv, pSrv)

	logger.Info().Str("addr", addr).Msg("serving grpc service")
	logger.Fatal().Err(gSrv.Serve(lis))

	// TODO: implement gRPC service
	logger.Fatal().Msg("gRPC server is not implemented")
}
