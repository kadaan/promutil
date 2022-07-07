package cmd

import (
	"github.com/kadaan/promutil/config"
	"github.com/kadaan/promutil/lib/exporter"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	outputFileKey = "output-file"
)

var (
	exportConfig config.ExportConfig

	// exportCmd represents the export command
	exportCmd = &cobra.Command{
		Use:   "export",
		Short: "Export prometheus data",
		Long:  `Export the specified data from a remote prometheus to a file.`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if !exportConfig.Start.Before(exportConfig.End) {
				return errors.New("start time is not before end time")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			err := exporter.Export(&exportConfig)
			if err != nil {
				return errors.Wrap(err, "export of data failed")
			}
			return nil
		},
	}
)

func init() {
	rootCmd.AddCommand(exportCmd)

	exportCmd.Flags().StringVar(&exportConfig.OutputFile, outputFileKey, config.DefaultOutputFile, "output file")
	_ = viper.BindPFlag(outputFileKey, exportCmd.Flags().Lookup(outputFileKey))

	exportCmd.Flags().Var(config.NewTimeValue(&exportConfig.Start, start), startKey, `time to start exporting from`)
	_ = viper.BindPFlag(startKey, exportCmd.Flags().Lookup(startKey))

	exportCmd.Flags().Var(config.NewTimeValue(&exportConfig.End, end), endKey, `time to export up through`)
	_ = viper.BindPFlag(endKey, exportCmd.Flags().Lookup(endKey))

	exportCmd.Flags().DurationVar(&exportConfig.SampleInterval, sampleIntervalKey, config.DefaultSampleInterval, "interval at which samples will be exported")
	_ = viper.BindPFlag(sampleIntervalKey, exportCmd.Flags().Lookup(sampleIntervalKey))

	exportCmd.Flags().StringArrayVar(&exportConfig.MatcherSetExpressions, matcherKey, config.DefaultMatcher, "set of matchers used to identify the data to export")
	_ = exportCmd.MarkFlagRequired(matcherKey)
	_ = viper.BindPFlag(matcherKey, exportCmd.Flags().Lookup(matcherKey))

	exportCmd.Flags().Var(config.NewSchemeValue(config.DefaultScheme, &exportConfig.Scheme), schemeKey, `scheme of the host server to export data from. allowed: "http", "https"`)
	_ = viper.BindPFlag(schemeKey, exportCmd.Flags().Lookup(schemeKey))

	exportCmd.Flags().StringVar(&exportConfig.Host, hostKey, config.DefaultHost, "host server to export data from")
	_ = viper.BindPFlag(hostKey, exportCmd.Flags().Lookup(hostKey))

	exportCmd.Flags().Uint16Var(&exportConfig.Port, portKey, config.DefaultPort, "host server to export data from")
	_ = viper.BindPFlag(portKey, exportCmd.Flags().Lookup(portKey))
}
