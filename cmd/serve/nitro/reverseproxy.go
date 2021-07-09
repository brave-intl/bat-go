package nitro

import (
	"github.com/brave-intl/bat-go/cmd"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/nitro"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// ReverseProxyServerCmd start up the grant server
	ReverseProxyServerCmd = &cobra.Command{
		Use:   "reverseproxy",
		Short: "subcommand to start up reverse proxy server",
		Run:   cmd.Perform("reverseproxy", RunReverseProxyServer),
	}
)

func init() {
	// upstream-url - sets the upstream-url of the server to be started
	ReverseProxyServerCmd.PersistentFlags().String("upstream-url", "",
		"the upstream url to proxy requests to")
	cmd.Must(ReverseProxyServerCmd.MarkPersistentFlagRequired("upstream-url"))
	cmd.Must(viper.BindPFlag("upstream-url", ReverseProxyServerCmd.PersistentFlags().Lookup("upstream-url")))
	cmd.Must(viper.BindEnv("upstream-url", "UPSTREAM_URL"))
}

func RunReverseProxyServer(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		return err
	}

	server, err := nitro.NewReverseProxyServer(
		viper.GetString("address"),
		viper.GetString("upstream-url"),
	)
	if err != nil {
		return err
	}

	logger.Info().
		Str("version", ctx.Value(appctx.VersionCTXKey).(string)).
		Str("commit", ctx.Value(appctx.CommitCTXKey).(string)).
		Str("build_time", ctx.Value(appctx.BuildTimeCTXKey).(string)).
		Str("upstream-url", viper.GetString("upstream-url")).
		Str("address", viper.GetString("address")).
		Str("environment", viper.GetString("environment")).
		Msg("server starting")
	return server.ListenAndServe()
}
