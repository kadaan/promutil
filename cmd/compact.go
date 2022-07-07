package cmd

import (
	"github.com/kadaan/promutil/config"
	"github.com/kadaan/promutil/lib/compactor"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	compactConfig config.CompactConfig

	// compactCmd represents the compact command
	compactCmd = &cobra.Command{
		Use:   "compact",
		Short: "Compact prometheus data",
		Long:  `Compact the specified prometheus data directory.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			err := compactor.Compact(&compactConfig)
			if err != nil {
				return errors.Wrap(err, "compact of data failed")
			}
			return nil
		},
	}
)

func init() {
	rootCmd.AddCommand(compactCmd)

	compactCmd.Flags().StringVar(&compactConfig.Directory, directoryKey, config.DefaultDataDirectory, "tsdb data directory to compact")
	_ = compactCmd.MarkFlagDirname(directoryKey)
	_ = viper.BindPFlag(directoryKey, compactCmd.Flags().Lookup(directoryKey))
}
