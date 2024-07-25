package cmd

import (

	// pprof imports
	_ "net/http/pprof"

	cmdutils "github.com/brave-intl/payments-service/cmd"
	srvcmd "github.com/brave-intl/payments-service/services/cmd"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	// setup the flags
	workersCmd.PersistentFlags().String("redis-addr", "127.0.0.1:6380", "the redis address")
	cmdutils.Must(viper.BindPFlag("redis-addr", workersCmd.PersistentFlags().Lookup("redis-addr")))
	cmdutils.Must(viper.BindEnv("redis-addr", "REDIS_ADDR"))

	workersCmd.PersistentFlags().String("redis-user", "", "the redis username")
	cmdutils.Must(viper.BindPFlag("redis-user", workersCmd.PersistentFlags().Lookup("redis-user")))
	cmdutils.Must(viper.BindEnv("redis-user", "REDIS_USER"))

	workersCmd.PersistentFlags().String("redis-pass", "", "the redis password")
	cmdutils.Must(viper.BindPFlag("redis-pass", workersCmd.PersistentFlags().Lookup("redis-pass")))
	cmdutils.Must(viper.BindEnv("redis-pass", "REDIS_PASS"))

	// add grpc and rest commands
	paymentsCmd.AddCommand(restCmd)

	// add worker command
	paymentsCmd.AddCommand(workersCmd)

	// add this command as a serve subcommand
	srvcmd.ServeCmd.AddCommand(paymentsCmd)
}

var (
	paymentsCmd = &cobra.Command{
		Use:   "payments",
		Short: "provides payments micro-service entrypoint",
	}

	restCmd = &cobra.Command{
		Use:   "rest",
		Short: "provides REST api services",
		Run:   RestRun,
	}

	workersCmd = &cobra.Command{
		Use:   "worker",
		Short: "provides redis stream worker",
		Run:   WorkerRun,
	}
)
