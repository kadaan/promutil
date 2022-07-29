package cmd

import (
	"github.com/kadaan/promutil/config"
	"github.com/kadaan/promutil/lib/command"
	"github.com/kadaan/promutil/lib/compactor"
)

func init() {
	command.NewCommand(
		Root,
		"compact",
		"Compact prometheus TSDB",
		"Compact the specified local prometheus TSDB.",
		new(config.CompactConfig),
		compactor.NewCompactor()).Configure(func(fb config.FlagBuilder, cfg *config.CompactConfig) {
		fb.Directory(&cfg.Directory, "directory read and write TSDB data")
	})
}
