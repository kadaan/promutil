package cmd

import (
	"github.com/kadaan/promutil/config"
	"github.com/kadaan/promutil/lib/backfiller"
	"github.com/kadaan/promutil/lib/block"
	"github.com/kadaan/promutil/lib/command"
)

func init() {
	command.NewCommand(
		Root,
		"backfill",
		"Backfill prometheus recording rule data",
		"Backfill prometheus recording rule data from the specified rules to a local prometheus TSDB.",
		new(config.BackfillConfig),
		backfiller.NewBackfiller()).Configure(func(fb config.FlagBuilder, cfg *config.BackfillConfig) {
		fb.TimeRange(&cfg.Start, &cfg.End, "time to backfill")
		fb.Directory(&cfg.Directory, "directory read and write TSDB data")
		fb.SampleInterval(&cfg.SampleInterval, "interval at which samples will be backfilled")
		fb.RecordingRules(&cfg.RuleConfig, "config file defining the rules to evaluate")
		fb.RuleGroupFilters(&cfg.RuleGroupFilters, "rule group filters which determine the rules groups to backfill")
		fb.RuleNameFilters(&cfg.RuleNameFilters, "rule name filters which determine the rules groups to backfill")
		fb.Parallelism(&cfg.Parallelism, block.MaxParallelism, "parallelism for backfill")
	})
}
