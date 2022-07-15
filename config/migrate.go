package config

import (
	"time"
)

// MigrateConfig represents the configuration of the migrate command.
type MigrateConfig struct {
	Scheme                Scheme
	Host                  string
	Port                  uint16
	Start                 time.Time
	End                   time.Time
	SampleInterval        time.Duration
	MatcherSetExpressions []string
	OutputDirectory       string
	Parallelism           uint8
}
