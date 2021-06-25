package nitro

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/utils/nitro"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// OpenProxyServerCmd start up the grant server
	OpenProxyServerCmd = &cobra.Command{
		Use:   "openproxy",
		Short: "subcommand to start up open proxy server",
		Run:   cmd.Perform("openproxy", RunOpenProxyServer),
	}
)

func RunOpenProxyServer(cmd *cobra.Command, args []string) error {
	addr := strings.Split(viper.GetString("address"), ":")
	if len(addr) != 2 {
		return fmt.Errorf("address must include port")
	}
	port, err := strconv.Atoi(addr[1])
	if err != nil || port < 0 {
		return fmt.Errorf("port must be a valid uint32: %v", err)
	}

	return nitro.ServeOpenProxy(
		uint32(port),
		10*time.Second,
	)
}
