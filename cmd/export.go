package cmd

import (
	"github.com/kadaan/promutil/config"
	"github.com/kadaan/promutil/lib"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	matcherKey         = "matcher"
	hostKey            = "host"
	portKey            = "port"
)

var (
	exportConfig config.ExportConfig

	// exportCmd represents the export command
	exportCmd = &cobra.Command{
		Use:   "export",
		Short: "Export prometheus data",
		Long: `Expose specific prometheus data to a columnar document.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			exporter, err := lib.NewExporter(&exportConfig)
			if err != nil {
				return errors.Wrap(err, "export of data failed")
			}
			return exporter.Export()
		},
	}
)

func init() {
	rootCmd.AddCommand(exportCmd)

	exportCmd.Flags().DurationVar(&exportConfig.Duration, durationKey, config.DefaultExportDuration, "duration of data to export")
	viper.BindPFlag(durationKey, exportCmd.Flags().Lookup(durationKey))

	exportCmd.Flags().DurationVar(&exportConfig.SampleInterval, sampleIntervalKey, config.DefaultExportSampleInterval, "interval at which samples will be exported")
	viper.BindPFlag(sampleIntervalKey, exportCmd.Flags().Lookup(sampleIntervalKey))

	exportCmd.Flags().StringArrayVar(&exportConfig.MatcherSetExpressions, matcherKey, []string{}, "set of matchers used to identify the data to export")
	exportCmd.MarkFlagRequired(matcherKey)
	viper.BindPFlag(matcherKey, exportCmd.Flags().Lookup(matcherKey))

	exportCmd.Flags().StringVar(&exportConfig.Host, hostKey, config.DefaultHost, "host server to export data from")
	viper.BindPFlag(hostKey, exportCmd.Flags().Lookup(hostKey))

	exportCmd.Flags().Uint16Var(&exportConfig.Port, portKey, config.DefaultPort, "host server to export data from")
	viper.BindPFlag(portKey, exportCmd.Flags().Lookup(portKey))
}