package nitro

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	rootcmd "github.com/brave-intl/bat-go/cmd"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/middleware"
	"github.com/brave-intl/bat-go/libs/nitro"
	srvcmd "github.com/brave-intl/bat-go/services/cmd"
	"github.com/brave-intl/bat-go/services/payments"
	chiware "github.com/go-chi/chi/middleware"
	"github.com/rs/zerolog/hlog"

	"github.com/go-chi/chi"
	"github.com/mdlayher/vsock"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	// upstream-url - sets the upstream-url of the server to be started
	NitroServeCmd.PersistentFlags().String("upstream-url", "", "the upstream url to proxy requests to")
	// egress-address - sets the vosck address for the open proxy to listen on for outgoing traffic
	NitroServeCmd.PersistentFlags().String("egress-address", "", "vsock address for open proxy to bind on")
	// log-address - sets the vosck address for the log server to listen on
	NitroServeCmd.PersistentFlags().String("log-address", "", "vsock address for log server to bind on")

	rootcmd.Must(NitroServeCmd.MarkPersistentFlagRequired("upstream-url"))
	rootcmd.Must(viper.BindPFlag("upstream-url", NitroServeCmd.PersistentFlags().Lookup("upstream-url")))
	rootcmd.Must(viper.BindEnv("upstream-url", "UPSTREAM_URL"))
	rootcmd.Must(NitroServeCmd.MarkPersistentFlagRequired("egress-address"))
	rootcmd.Must(viper.BindPFlag("egress-address", NitroServeCmd.PersistentFlags().Lookup("egress-address")))
	rootcmd.Must(viper.BindEnv("egress-address", "EGRESS_ADDRESS"))
	rootcmd.Must(NitroServeCmd.MarkPersistentFlagRequired("log-address"))
	rootcmd.Must(viper.BindPFlag("log-address", NitroServeCmd.PersistentFlags().Lookup("log-address")))
	rootcmd.Must(viper.BindEnv("log-address", "LOG_ADDRESS"))

	// enclave decrypt key template used to create decryption key in enclave
	rootcmd.Must(viper.BindPFlag("enclave-decrypt-key-template-secret", NitroServeCmd.PersistentFlags().Lookup("enclave-decrypt-key-template-secret")))
	rootcmd.Must(viper.BindEnv("enclave-decrypt-key-template-secret", "ENCLAVE_DECRYPT_KEY_TEMPLATE_SECRET"))

	NitroServeCmd.AddCommand(OutsideNitroServeCmd)
	NitroServeCmd.AddCommand(InsideNitroServeCmd)
	srvcmd.ServeCmd.AddCommand(NitroServeCmd)
}

// OutsideNitroServeCmd the nitro serve command
var OutsideNitroServeCmd = &cobra.Command{
	Use:   "outside-enclave",
	Short: "subcommand to serve a nitro micro-service",
	Run:   rootcmd.Perform("outside-enclave", RunNitroServerOutsideEnclave),
}

// InsideNitroServeCmd the nitro serve command
var InsideNitroServeCmd = &cobra.Command{
	Use:   "inside-enclave",
	Short: "subcommand to serve a nitro micro-service",
	Run:   rootcmd.Perform("inside-enclave", RunNitroServerInEnclave),
}

// NitroServeCmd the nitro serve command
var NitroServeCmd = &cobra.Command{
	Use:   "nitro",
	Short: "subcommand to serve a nitro micro-service",
}

// RunNitroServerInEnclave - start up the nitro server living inside the enclave
func RunNitroServerInEnclave(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	logaddr := viper.GetString("log-address")
	writer := nitro.NewVsockWriter(logaddr)
	ctx = context.WithValue(ctx, appctx.LogWriterCTXKey, writer)
	ctx = context.WithValue(ctx, appctx.EgressProxyAddrCTXKey, viper.GetString("egress-address"))
	ctx = context.WithValue(ctx, appctx.EnclaveDecryptKeyTemplateSecretIDCTXKey, viper.GetString("enclave-decrypt-key-template-secret"))
	// special logger with writer
	ctx, logger := logging.SetupLogger(ctx)
	// setup the service now
	ctx, s, err := payments.NewService(ctx)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to initialize payments service")
	}
	logger.Info().Msg("payments service setup")
	// setup router
	ctx, r := setupRouter(ctx, s)
	logger.Info().Msg("payments routes setup")

	// setup listener
	addr := viper.GetString("address")
	port, err := strconv.ParseUint(strings.Split(addr, ":")[1], 10, 32)
	if err != nil || port != 0 {
		// panic if there is an error, or if the port is too large to fit in uint32
		logger.Panic().Err(err).Msg("invalid --address")
	}

	// setup vsock listener
	l, err := vsock.Listen(uint32(port), &vsock.Config{})
	if err != nil {
		logger.Panic().Err(err).Msg("listening on vsock port failed")
	}
	logger.Info().Msg("vsock listener setup")
	// setup server
	srv := http.Server{
		Handler:      chi.ServerBaseContext(ctx, r),
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 20 * time.Second,
	}
	logger.Info().Msg("starting server")
	// run the server in another routine
	logger.Fatal().Err(srv.Serve(l)).Msg("server shutdown")
	return nil
}

func setupRouter(ctx context.Context, s *payments.Service) (context.Context, *chi.Mux) {
	// base service logger
	logger := logging.Logger(ctx, "payments")
	// base router
	r := chi.NewRouter()
	// middlewares
	r.Use(chiware.RequestID)
	r.Use(middleware.RequestIDTransfer)
	r.Use(hlog.NewHandler(*logger))
	r.Use(hlog.UserAgentHandler("user_agent"))
	r.Use(hlog.RequestIDHandler("req_id", "Request-Id"))
	r.Use(middleware.RequestLogger(logger))
	r.Use(chiware.Timeout(15 * time.Second))
	r.Use(s.ConfigurationMiddleware)
	logger.Info().Msg("configuration middleware setup")
	// routes
	r.Method("GET", "/", http.HandlerFunc(nitro.EnclaveHealthCheck))
	r.Method("GET", "/health-check", http.HandlerFunc(nitro.EnclaveHealthCheck))
	// setup payments routes
	// prepare inserts transactions into qldb, returning a document which needs to be submitted by an authorizer
	r.Post("/v1/payments/prepare", middleware.InstrumentHandler("PrepareHandler", payments.PrepareHandler(s)).ServeHTTP)
	logger.Info().Msg("prepare endpoint setup")
	// submit will have an http signature from a known list of public keys
	r.Post("/v1/payments/submit", middleware.InstrumentHandler("SubmitHandler", s.AuthorizerSignedMiddleware()(payments.SubmitHandler(s))).ServeHTTP)
	logger.Info().Msg("submit endpoint setup")

	r.Get("/v1/configuration", handlers.AppHandler(payments.GetConfigurationHandler( /*s*/ )).ServeHTTP)
	logger.Info().Msg("get config endpoint setup")
	return ctx, r
}

// RunNitroServerOutsideEnclave - start up all the services which are outside
func RunNitroServerOutsideEnclave(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		return err
	}

	egressaddr := strings.Split(viper.GetString("egress-address"), ":")
	if len(egressaddr) != 2 {
		return fmt.Errorf("address must include port")
	}
	egressport, err := strconv.ParseUint(egressaddr[1], 10, 32)
	if err != nil || egressport == 0 {
		return fmt.Errorf("port must be a valid uint32 and not 0: %v", err)
	}

	logaddr := strings.Split(viper.GetString("log-address"), ":")
	if len(logaddr) != 2 {
		return fmt.Errorf("address must include port")
	}
	logport, err := strconv.ParseUint(logaddr[1], 10, 32)
	if err != nil || logport == 0 {
		return fmt.Errorf("port must be a valid uint32 and not 0: %v", err)
	}
	logserve := nitro.NewVsockLogServer(ctx, uint32(logport))

	logger.Info().
		Str("version", ctx.Value(appctx.VersionCTXKey).(string)).
		Str("commit", ctx.Value(appctx.CommitCTXKey).(string)).
		Str("build_time", ctx.Value(appctx.BuildTimeCTXKey).(string)).
		Str("upstream-url", viper.GetString("upstream-url")).
		Str("address", viper.GetString("address")).
		Str("environment", viper.GetString("environment")).
		Msg("server starting")

	done := make(chan struct{})

	// open proxy server
	logger.Info().
		Msg("starting serve open proxy")

	go func() {
		if err := nitro.ServeOpenProxy(
			ctx,
			uint32(egressport),
			10*time.Second,
		); err != nil {
			logger.Fatal().Err(err).Msg("failed to start open proxy")
		}
	}()

	go func() {
		if err := logserve.Serve(nil); err != nil {
			logger.Fatal().Err(err).Msg("failed to start log server")
		}
	}()

	logger.Info().Msg("startup complete for libs")

	// wait forever
	<-done
	return nil
}
