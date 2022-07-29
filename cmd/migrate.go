package cmd

import (
	"github.com/kadaan/promutil/config"
	"github.com/kadaan/promutil/lib/command"
	"github.com/kadaan/promutil/lib/migrator"
)

func init() {
	command.NewCommand(
		Root,
		"migrate",
		"Migrate prometheus data",
		"Migrate the specified data from a remote prometheus to a local prometheus TSDB.",
		new(config.MigrateConfig),
		migrator.NewMigrator()).Configure(func(fb config.FlagBuilder, cfg *config.MigrateConfig) {
		fb.TimeRange(&cfg.Start, &cfg.End, "time to migrate")
		fb.OutputDirectory(&cfg.OutputDirectory, "directory write TSDB data")
		fb.SampleInterval(&cfg.SampleInterval, "interval at which samples will be migrated")
		fb.Matchers(&cfg.Matchers, "config file defining the rules to evaluate")
		fb.Host(&cfg.Host, "remote host to migrate data from")
		fb.Parallelism(&cfg.Parallelism, 4, "parallelism for migration")
	})
}
