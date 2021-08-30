package payments

import (
	"context"
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

	logger.Debug().
		Str("datastore", viper.GetString("datastore")).
		Msg("setting up payments datastore")

	datastore, err := payments.NewPostgres(viper.GetString("datastore"), true, "payments", "payments")
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to setup datastore")
	}

	ctx = context.WithValue(ctx, appctx.DatastoreCTXKey, datastore)

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
	/*
		var opts []grpc.ServerOption
			creds, err := credentials.NewServerTLSFromFile(
				viper.GetString("cert"), viper.GetString("cert-key"))
			if err != nil {
				logger.Fatal().Err(err).Msg("failed to set credentials for server")
			}
			opts = []grpc.ServerOption{grpc.Creds(creds)}
	*/

	gSrv := grpc.NewServer(grpc.UnaryInterceptor(setContextInterceptor(ctx)))

	paymentsPB.RegisterPaymentsGRPCServiceServer(gSrv, pSrv)

	logger.Info().Str("addr", addr).Msg("serving grpc service")
	logger.Fatal().Err(gSrv.Serve(lis))
}

func setContextInterceptor(c context.Context) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// merge ctx
		return handler(appctx.Wrap(ctx, c), req)
	}
}
