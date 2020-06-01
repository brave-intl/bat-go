package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	rootCmd.AddCommand(serveCmd)

	// env - defaults to development
	serveCmd.PersistentFlags().StringVarP(&address, "address", "a", ":8080",
		"the default address to bind to")
	must(viper.BindPFlag("address", serveCmd.PersistentFlags().Lookup("address")))
	must(viper.BindEnv("address", "ADDR"))
}

var address string

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "entrypoint to serve a micro-service",
}
