package nitro

import (
	"context"
	"fmt"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	rootcmd "github.com/brave-intl/payments-service/cmd"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/nitro"
	srvcmd "github.com/brave-intl/payments-service/services/cmd"
	"github.com/brave-intl/payments-service/services/payments"

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
	// enclave decrypt key template secret
	NitroServeCmd.PersistentFlags().String("enclave-decrypt-key-template-secret", "", "the template secret which has the decrypt key policy")
	// aws-region - sets the aws region used for SDK integration
	NitroServeCmd.PersistentFlags().String("aws-region", "", "the aws region used for SDK integration")
	// qldb-role-arn - sets the AWS ARN for the role to use to access QLDB
	NitroServeCmd.PersistentFlags().String("qldb-role-arn", "", "the AWS ARN for the role to use to access QLDB")
	// qldb-ledger-name - sets the QLDB ledger name to use
	NitroServeCmd.PersistentFlags().String("qldb-ledger-name", "", "the QLDB ledger name to use")
	// qldb-ledger-arn - sets the AWS ARN for the QLDB ledger
	NitroServeCmd.PersistentFlags().String("qldb-ledger-arn", "", "the AWS ARN for the QLDB ledger")

	// enclave-config-object-name is the config object name in s3 (from configure command)
	NitroServeCmd.PersistentFlags().String("enclave-config-object-name", "", "the configuration object name in s3")
	// enclave-config-bucket-name is the config bucket name in s3 (from configure command)
	NitroServeCmd.PersistentFlags().String("enclave-config-bucket-name", "", "the configuration bucket name in s3")
	// enclave-operator-shares-bucket-name is the operator-shares bucket name in s3 (from bootstrap command)
	NitroServeCmd.PersistentFlags().String("enclave-operator-shares-bucket-name", "", "the operator shares bucket name in s3")
	// enclave-solana-address-name is the solana address to use for payouts (from configure command)
	NitroServeCmd.PersistentFlags().String("enclave-solana-address-name", "", "the solana address to use for payouts")

	rootcmd.Must(NitroServeCmd.MarkPersistentFlagRequired("upstream-url"))
	rootcmd.Must(viper.BindPFlag("upstream-url", NitroServeCmd.PersistentFlags().Lookup("upstream-url")))
	rootcmd.Must(viper.BindEnv("upstream-url", "UPSTREAM_URL"))
	rootcmd.Must(NitroServeCmd.MarkPersistentFlagRequired("egress-address"))
	rootcmd.Must(viper.BindPFlag("egress-address", NitroServeCmd.PersistentFlags().Lookup("egress-address")))
	rootcmd.Must(viper.BindEnv("egress-address", "EGRESS_ADDRESS"))
	rootcmd.Must(NitroServeCmd.MarkPersistentFlagRequired("log-address"))
	rootcmd.Must(viper.BindPFlag("log-address", NitroServeCmd.PersistentFlags().Lookup("log-address")))
	rootcmd.Must(viper.BindEnv("log-address", "LOG_ADDRESS"))

	rootcmd.Must(viper.BindPFlag("aws-region", NitroServeCmd.PersistentFlags().Lookup("aws-region")))
	rootcmd.Must(viper.BindEnv("aws-region", "AWS_REGION"))
	rootcmd.Must(viper.BindPFlag("qldb-role-arn", NitroServeCmd.PersistentFlags().Lookup("qldb-role-arn")))
	rootcmd.Must(viper.BindEnv("qldb-role-arn", "QLDB_ROLE_ARN"))
	rootcmd.Must(viper.BindPFlag("qldb-ledger-name", NitroServeCmd.PersistentFlags().Lookup("qldb-ledger-name")))
	rootcmd.Must(viper.BindEnv("qldb-ledger-name", "QLDB_LEDGER_NAME"))
	rootcmd.Must(viper.BindPFlag("qldb-ledger-arn", NitroServeCmd.PersistentFlags().Lookup("qldb-ledger-arn")))
	rootcmd.Must(viper.BindEnv("qldb-ledger-arn", "QLDB_LEDGER_ARN"))

	rootcmd.Must(viper.BindPFlag("enclave-config-object-name", NitroServeCmd.PersistentFlags().Lookup("enclave-config-object-name")))
	rootcmd.Must(viper.BindEnv("enclave-config-object-name", "ENCLAVE_CONFIG_OBJECT_NAME"))
	rootcmd.Must(viper.BindPFlag("enclave-config-bucket-name", NitroServeCmd.PersistentFlags().Lookup("enclave-config-bucket-name")))
	rootcmd.Must(viper.BindEnv("enclave-config-bucket-name", "ENCLAVE_CONFIG_BUCKET_NAME"))
	rootcmd.Must(viper.BindPFlag("enclave-operator-shares-bucket-name", NitroServeCmd.PersistentFlags().Lookup("enclave-operator-shares-bucket-name")))
	rootcmd.Must(viper.BindEnv("enclave-operator-shares-bucket-name", "ENCLAVE_OPERATOR_SHARES_BUCKET_NAME"))
	rootcmd.Must(viper.BindPFlag("enclave-solana-address-name", NitroServeCmd.PersistentFlags().Lookup("enclave-config-object-name")))
	rootcmd.Must(viper.BindEnv("enclave-solana-address-name", "ENCLAVE_SOLANA_ADDRESS_NAME"))

	// enclave decrypt key template used to create decryption key in enclave
	viper.BindPFlag("enclave-decrypt-key-template-secret", NitroServeCmd.PersistentFlags().Lookup("enclave-decrypt-key-template-secret"))
	viper.BindEnv("enclave-decrypt-key-template-secret", "ENCLAVE_DECRYPT_KEY_TEMPLATE_SECRET")

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
	ctx = context.WithValue(ctx, appctx.AWSRegionCTXKey, viper.GetString("aws-region"))
	ctx = context.WithValue(ctx, appctx.PaymentsQLDBRoleArnCTXKey, viper.GetString("qldb-role-arn"))
	ctx = context.WithValue(ctx, appctx.PaymentsQLDBLedgerNameCTXKey, viper.GetString("qldb-ledger-name"))
	ctx = context.WithValue(ctx, appctx.PaymentsQLDBLedgerARNCTXKey, viper.GetString("qldb-ledger-arn"))
	ctx = context.WithValue(ctx, appctx.EnclaveDecryptKeyTemplateSecretIDCTXKey, viper.GetString("enclave-decrypt-key-template-secret"))

	ctx = context.WithValue(ctx, appctx.EnclaveSecretsObjectNameCTXKey, viper.GetString("enclave-config-object-name"))
	ctx = context.WithValue(ctx, appctx.EnclaveSolanaAddressCTXKey, viper.GetString("enclave_solana_address_name"))
	ctx = context.WithValue(ctx, appctx.EnclaveSecretsBucketNameCTXKey, viper.GetString("enclave-config-bucket-name"))
	ctx = context.WithValue(ctx, appctx.EnclaveOperatorSharesBucketNameCTXKey, viper.GetString("enclave-operator-shares-bucket-name"))
	// special logger with writer
	ctx, logger := logging.SetupLogger(ctx)

	// setup panic handler so we send details to our remote logger
	defer func() {
		if rec := recover(); rec != nil {
			// report the reason for the panic
			logger.Error().
				Str("panic", fmt.Sprintf("%+v", rec)).
				Str("stacktrace", string(debug.Stack())).
				Msg("panic recovered")
		}
	}()

	logger.Info().Msg("starting payments service")
	// setup the service now
	ctx, s, err := payments.NewService(ctx)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to initialize payments service")
	}
	logger.Info().Msg("payments service setup")

	// setup router
	ctx, r := payments.SetupRouter(ctx, s)
	logger.Info().Msg("payments routes setup")

	// setup listener
	addr := viper.GetString("address")
	port, err := strconv.ParseUint(strings.Split(addr, ":")[1], 10, 32)
	if err != nil || port == 0 {
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
