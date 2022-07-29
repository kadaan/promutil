package config

import "time"

const (
	DefaultMetricConfigFile = ""
)

// GenerateConfig represents the configuration of the generate command.
type GenerateConfig struct {
	Start           time.Time
	End             time.Time
	OutputDirectory string
	SampleInterval  time.Duration
	MetricConfig    MetricConfig
	RuleConfig      RecordingRules
	Parallelism     uint8
}
