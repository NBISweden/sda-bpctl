package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:          "bpctl",
	Short:        "Runs dataset submissions",
	Long:         `Runs dataset submissions`,
	SilenceUsage: true,
}

func Execute() error {
	err := rootCmd.Execute()
	if err != nil {
		return err
	}

	return nil
}

func AddCommand(command *cobra.Command) {
	rootCmd.AddCommand(command)
}
