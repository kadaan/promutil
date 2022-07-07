package cmd

import (
	"github.com/kadaan/promutil/config"
	"github.com/kadaan/promutil/lib/migrator"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	DefaultMigrateParallelism = 4
)

var (
	migrateConfig config.MigrateConfig

	// migrateCmd represents the migrate command
	migrateCmd = &cobra.Command{
		Use:   "migrate",
		Short: "Migrate prometheus data",
		Long:  `Migrate the specified data from a remote prometheus to a local prometheus.`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if !migrateConfig.Start.Before(migrateConfig.End) {
				return errors.New("start time is not before end time")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			err := migrator.Migrate(&migrateConfig)
			if err != nil {
				return errors.Wrap(err, "migrate of data failed")
			}
			return nil
		},
	}
)

func init() {
	rootCmd.AddCommand(migrateCmd)

	migrateCmd.Flags().StringVar(&migrateConfig.OutputDirectory, outputDirectoryKey, config.DefaultDataDirectory, "output directory to write tsdb data")
	_ = migrateCmd.MarkFlagDirname(outputDirectoryKey)
	_ = viper.BindPFlag(outputDirectoryKey, migrateCmd.Flags().Lookup(outputDirectoryKey))

	migrateCmd.Flags().Var(config.NewTimeValue(&migrateConfig.Start, start), startKey, `time to start migrating from`)
	_ = viper.BindPFlag(startKey, migrateCmd.Flags().Lookup(startKey))

	migrateCmd.Flags().Var(config.NewTimeValue(&migrateConfig.End, end), endKey, `time to migrate up through from`)
	_ = viper.BindPFlag(endKey, migrateCmd.Flags().Lookup(endKey))

	migrateCmd.Flags().DurationVar(&migrateConfig.SampleInterval, sampleIntervalKey, config.DefaultSampleInterval, "interval at which samples will be migrated")
	_ = viper.BindPFlag(sampleIntervalKey, migrateCmd.Flags().Lookup(sampleIntervalKey))

	migrateCmd.Flags().StringArrayVar(&migrateConfig.MatcherSetExpressions, matcherKey, config.DefaultMatcher, "set of matchers used to identify the data to migrated")
	_ = migrateCmd.MarkFlagRequired(matcherKey)
	_ = viper.BindPFlag(matcherKey, migrateCmd.Flags().Lookup(matcherKey))

	migrateCmd.Flags().VarP(config.NewSchemeValue(config.DefaultScheme, &migrateConfig.Scheme), schemeKey, "", `scheme of the host server to export data from. allowed: "http", "https"`)
	_ = viper.BindPFlag(schemeKey, migrateCmd.Flags().Lookup(schemeKey))

	migrateCmd.Flags().StringVar(&migrateConfig.Host, hostKey, config.DefaultHost, "host server to migrate data from")
	_ = viper.BindPFlag(hostKey, migrateCmd.Flags().Lookup(hostKey))

	migrateCmd.Flags().Uint16Var(&migrateConfig.Port, portKey, config.DefaultPort, "host server to migrate data from")
	_ = viper.BindPFlag(portKey, migrateCmd.Flags().Lookup(portKey))

	migrateCmd.Flags().Uint8Var(&migrateConfig.Parallelism, parallelismKey, DefaultMigrateParallelism, "parallelism for migration")
	_ = viper.BindPFlag(parallelismKey, migrateCmd.Flags().Lookup(parallelismKey))
}
