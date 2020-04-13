package config

import "time"

const (
	// DefaultGenerateDuration is the default data generation duration.
	DefaultGenerateDuration = time.Hour * 720

	// DefaultGenerateOutputDirectory is the default directory to write the TSDB.
	DefaultGenerateOutputDirectory = "data/"

	// DefaultGenerateSampleInterval is the default interval of generated metric samples.
	DefaultGenerateSampleInterval = time.Second * 15

	// DefaultGenerateBlockLength is the default length of generated TSDB blocks.
	DefaultGenerateBlockLength = time.Second * 15
)

// GenerateConfig represents the configuration of the generate command.
type GenerateConfig struct {
	Duration         time.Duration
	OutputDirectory  string
	SampleInterval   time.Duration
	BlockLength      time.Duration
	MetricConfigFile string
	MetricConfig     *MetricConfig
	RuleConfigFiles  []string
	RuleConfig       RecordingRules
}