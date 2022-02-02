package nitro

import (
	"context"
	"crypto/rand"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/cmd"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/nitro"
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

	cmd.Must(NitroServeCmd.MarkPersistentFlagRequired("upstream-url"))
	cmd.Must(viper.BindPFlag("upstream-url", NitroServeCmd.PersistentFlags().Lookup("upstream-url")))
	cmd.Must(viper.BindEnv("upstream-url", "UPSTREAM_URL"))
	cmd.Must(NitroServeCmd.MarkPersistentFlagRequired("egress-address"))
	cmd.Must(viper.BindPFlag("egress-address", NitroServeCmd.PersistentFlags().Lookup("egress-address")))
	cmd.Must(viper.BindEnv("egress-address", "EGRESS_ADDRESS"))
	cmd.Must(NitroServeCmd.MarkPersistentFlagRequired("log-address"))
	cmd.Must(viper.BindPFlag("log-address", NitroServeCmd.PersistentFlags().Lookup("log-address")))
	cmd.Must(viper.BindEnv("log-address", "LOG_ADDRESS"))

	cmd.ServeCmd.AddCommand(NitroServeCmd)
}

// NitroServeCmd the nitro serve command
var NitroServeCmd = &cobra.Command{
	Use:   "nitro",
	Short: "subcommand to serve a nitro micro-service",
	Run:   cmd.Perform("nitro", RunNitroServer),
}

func RunNitroServer(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	c := make(chan int, 1)

	go func() {
		b := make([]byte, 1)
		n, err := rand.Read(b)
		if err != nil {
			logger, err := appctx.GetLogger(ctx)
			if err != nil {
				logger.Error().Err(err).Msg("Unexpected error while reading rand")
			}
		}
		c <- n
	}()
	select {
	case <-c:
		return RunNitroServerOutsideEnclave(cmd, args)
	case <-time.After(1 * time.Second):
		return RunNitroServerInEnclave(cmd, args)
	}
}

func RunNitroServerInEnclave(cmd *cobra.Command, args []string) error {
	fmt.Println("running inside encalve")
	ctx := cmd.Context()

	logaddr := viper.GetString("log-address")

	writer := nitro.NewVsockWriter(logaddr)
	ctx = context.WithValue(ctx, appctx.LogWriterKey, writer)

	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		return err
	}

	logger.Info().Msg("Last of the enclaves")
	return nil
}

func RunNitroServerOutsideEnclave(cmd *cobra.Command, args []string) error {
	fmt.Println("running outside encalve")
	ctx := cmd.Context()
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		return err
	}

	egressaddr := strings.Split(viper.GetString("egress-address"), ":")
	fmt.Println(egressaddr)
	if len(egressaddr) != 2 {
		return fmt.Errorf("address must include port")
	}
	egressport, err := strconv.Atoi(egressaddr[1])
	if err != nil || egressport < 0 {
		return fmt.Errorf("port must be a valid uint32: %v", err)
	}

	server, err := nitro.NewReverseProxyServer(
		viper.GetString("address"),
		viper.GetString("upstream-url"),
	)
	if err != nil {
		return err
	}

	logaddr := strings.Split(viper.GetString("log-address"), ":")
	if len(logaddr) != 2 {
		return fmt.Errorf("address must include port")
	}
	logport, err := strconv.Atoi(logaddr[1])
	if err != nil || logport < 0 {
		return fmt.Errorf("port must be a valid uint32: %v", err)
	}
	logserve := nitro.NewVsockLogServer(logport)

	logger.Info().
		Str("version", ctx.Value(appctx.VersionCTXKey).(string)).
		Str("commit", ctx.Value(appctx.CommitCTXKey).(string)).
		Str("build_time", ctx.Value(appctx.BuildTimeCTXKey).(string)).
		Str("upstream-url", viper.GetString("upstream-url")).
		Str("address", viper.GetString("address")).
		Str("environment", viper.GetString("environment")).
		Msg("server starting")

	go logserve.Serve(nil)

	go nitro.ServeOpenProxy(
		uint32(egressport),
		10*time.Second,
	)

	return server.ListenAndServe()
}
