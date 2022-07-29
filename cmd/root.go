package cmd

import (
	"github.com/kadaan/promutil/lib/command"
	"github.com/kadaan/promutil/version"
)

var (
	Root = command.NewRootCommand(
		"prometheus utilities",
		version.Name+` provides a set of utilities for working with a Prometheus 
TSDB.  It allows data generation, recording rule backfilling, data 
migration, etc.`)
)
