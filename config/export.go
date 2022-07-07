package config

import (
	"time"
)

const (
	DefaultOutputFile = "data.json.zst"
)

// ExportConfig represents the configuration of the export command.
type ExportConfig struct {
	Scheme                Scheme
	Host                  string
	Port                  uint16
	Start                 time.Time
	End                   time.Time
	SampleInterval        time.Duration
	MatcherSetExpressions []string
	OutputFile            string
}
