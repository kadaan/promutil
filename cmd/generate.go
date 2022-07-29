package cmd

import (
	"github.com/kadaan/promutil/config"
	"github.com/kadaan/promutil/lib/block"
	"github.com/kadaan/promutil/lib/command"
	"github.com/kadaan/promutil/lib/generator"
)

func init() {
	command.NewCommand(
		Root,
		"generate",
		"Generate prometheus data",
		"Generate prometheus data based on the provided data definitions to a local prometheus TSDB.",
		new(config.GenerateConfig),
		generator.NewGenerator()).Configure(func(fb config.FlagBuilder, cfg *config.GenerateConfig) {
		fb.TimeRange(&cfg.Start, &cfg.End, "time to generate data")
		fb.OutputDirectory(&cfg.OutputDirectory, "output directory to write TSDB data")
		fb.SampleInterval(&cfg.SampleInterval, "interval at which samples will be generated")
		fb.MetricConfig(&cfg.MetricConfig, "config file defining the time series to create").Required()
		fb.RecordingRules(&cfg.RuleConfig, "config file defining the rules to evaluate")
		fb.Parallelism(&cfg.Parallelism, block.MaxParallelism, "parallelism for generation")
	})
}
