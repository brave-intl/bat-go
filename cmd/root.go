package cmd

import (
	"log"
	"os"

	"github.com/spf13/cobra"
)

var (
	rootCmd = &cobra.Command{
		Use:   "bat-go",
		Short: "bat-go provides go based services and processes for BAT",
	}
)

// Execute - the main entrypoint for all subcommands in bat-go
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		log.Printf("error running bat-go: %s\n", err.Error())
		os.Exit(1)
	}
}
