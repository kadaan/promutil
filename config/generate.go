package config

import "time"

const (
	DefaultMetricConfigFile = ""
)

// GenerateConfig represents the configuration of the generate command.
type GenerateConfig struct {
	Start            time.Time
	End              time.Time
	OutputDirectory  string
	SampleInterval   time.Duration
	BlockLength      time.Duration
	MetricConfigFile string
	MetricConfig     *MetricConfig
	RuleConfigFiles  []string
	RuleConfig       RecordingRules
}
