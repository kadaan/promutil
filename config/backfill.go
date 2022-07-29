package config

import (
	"regexp"
	"time"
)

// BackfillConfig represents the configuration of the backfill command.
type BackfillConfig struct {
	Start            time.Time
	End              time.Time
	SampleInterval   time.Duration
	RuleConfig       RecordingRules
	RuleGroupFilters []*regexp.Regexp
	RuleNameFilters  []*regexp.Regexp
	Directory        string
	Parallelism      uint8
}
