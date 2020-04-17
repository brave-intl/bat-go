package cmd

import "github.com/spf13/cobra"

func init() {
	rootCmd.AddCommand(settlementCmd)
}

var settlementCmd = &cobra.Command{
	Use:   "settlement",
	Short: "provides settlement utilities",
}
