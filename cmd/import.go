package cmd

import (
	"github.com/kadaan/promutil/config"
	"github.com/kadaan/promutil/lib/importer"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	dataFileKey = "data-file"
)

var (
	importConfig config.ImportConfig

	// importCmd represents the import command
	importCmd = &cobra.Command{
		Use:   "import",
		Short: "Import prometheus data",
		Long:  `Import prometheus data from a file.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			err := importer.Import(&importConfig)
			if err != nil {
				return errors.Wrap(err, "import of data failed")
			}
			return nil
		},
	}
)

func init() {
	rootCmd.AddCommand(importCmd)

	importCmd.Flags().StringVar(&importConfig.OutputDirectory, outputDirectoryKey, config.DefaultDataDirectory, "output directory to write tsdb data")
	_ = importCmd.MarkFlagDirname(outputDirectoryKey)
	_ = viper.BindPFlag(outputDirectoryKey, importCmd.Flags().Lookup(outputDirectoryKey))

	importCmd.Flags().StringArrayVar(&importConfig.DataFiles, dataFileKey, []string{}, "file containing the data to import")
	_ = importCmd.MarkFlagFilename(dataFileKey, "yml", "yaml")
	_ = viper.BindPFlag(dataFileKey, importCmd.Flags().Lookup(dataFileKey))
}
