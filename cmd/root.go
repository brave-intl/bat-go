package cmd

import (
	"context"
	"log"
	"os"

	"github.com/spf13/cobra"
)

var (
	ctx     = context.Background()
	rootCmd = &cobra.Command{
		Use:   "bat-go",
		Short: "bat-go provides go based services and processes for BAT",
	}
)

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		log.Printf("error running bat-go: %s\n", err.Error())
		os.Exit(1)
	}
}
