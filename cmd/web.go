package cmd

import (
	"github.com/kadaan/promutil/config"
	"github.com/kadaan/promutil/lib/command"
	"github.com/kadaan/promutil/lib/web"
)

func init() {
	command.NewCommand(
		Root,
		"web",
		"Runs an API/UI server",
		"Runs an API/UI server for web based prometheus utilities.",
		new(config.WebConfig),
		web.NewWeb()).Configure(func(fb config.FlagBuilder, cfg *config.WebConfig) {
		fb.ListenAddress(&cfg.ListenAddress, "the listen address")
		fb.SampleInterval(&cfg.SampleInterval, "interval at which samples will be taken within a range")
		fb.Host(&cfg.Host, "remote prometheus host")
	})
}
