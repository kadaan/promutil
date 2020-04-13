package cmd

import (
	"github.com/kadaan/promutil/version"
	"github.com/spf13/cobra"
)

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Prints the promutil version",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Printf(version.Print())
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
