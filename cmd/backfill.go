package cmd

import (
	"github.com/kadaan/promutil/config"
	"github.com/kadaan/promutil/lib/backfiller"
	"github.com/kadaan/promutil/lib/block"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	backfillConfig config.BackfillConfig

	// backfillCmd represents the backfill command
	backfillCmd = &cobra.Command{
		Use:   "backfill",
		Short: "Backfill prometheus data",
		Long:  `Backfill prometheus data from the specified rule to a local prometheus.`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if !backfillConfig.Start.Before(backfillConfig.End) {
				return errors.New("start time is not before end time")
			}
			return nil

		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := backfiller.Backfill(&backfillConfig); err != nil {
				return errors.Wrap(err, "backfill of data failed")
			}
			return nil
		},
	}
)

func init() {
	rootCmd.AddCommand(backfillCmd)

	backfillCmd.Flags().StringVar(&backfillConfig.OutputDirectory, directoryKey, config.DefaultDataDirectory, "directory read and write tsdb data")
	_ = migrateCmd.MarkFlagDirname(directoryKey)
	_ = viper.BindPFlag(directoryKey, backfillCmd.Flags().Lookup(directoryKey))

	backfillCmd.Flags().Var(config.NewTimeValue(&backfillConfig.Start, start), startKey, `time to start backfill from`)
	_ = viper.BindPFlag(startKey, backfillCmd.Flags().Lookup(startKey))

	backfillCmd.Flags().Var(config.NewTimeValue(&backfillConfig.End, end), endKey, `time to backfill up through from`)
	_ = viper.BindPFlag(endKey, backfillCmd.Flags().Lookup(endKey))

	backfillCmd.Flags().DurationVar(&backfillConfig.SampleInterval, sampleIntervalKey, config.DefaultSampleInterval, "interval at which samples will be backfill")
	_ = viper.BindPFlag(sampleIntervalKey, backfillCmd.Flags().Lookup(sampleIntervalKey))

	backfillCmd.Flags().Var(config.NewRecordingRulesValue(&backfillConfig.RuleConfig), ruleConfigFileKey, "config file defining the rules to evaluate")
	_ = backfillCmd.MarkFlagFilename(ruleConfigFileKey, config.YamlFileExtensions...)
	_ = viper.BindPFlag(ruleConfigFileKey, backfillCmd.Flags().Lookup(ruleConfigFileKey))

	backfillCmd.Flags().Var(config.NewRegexValue(&backfillConfig.RuleGroupFilters, config.DefaultRuleGroupFilters), ruleGroupFilterKey, "rule group filters which determine the rules groups to backfill")
	_ = viper.BindPFlag(ruleGroupFilterKey, backfillCmd.Flags().Lookup(ruleGroupFilterKey))

	backfillCmd.Flags().Var(config.NewRegexValue(&backfillConfig.RuleNameFilters, config.DefaultRuleNameFilters), ruleNameFilterKey, "rule name filters which determine the rules groups to backfill")
	_ = viper.BindPFlag(ruleNameFilterKey, backfillCmd.Flags().Lookup(ruleNameFilterKey))

	backfillCmd.Flags().Uint8Var(&backfillConfig.Parallelism, parallelismKey, block.MaxParallelism, "parallelism for migration")
	_ = viper.BindPFlag(parallelismKey, backfillCmd.Flags().Lookup(parallelismKey))
}
