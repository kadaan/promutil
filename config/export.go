package config

import (
	"time"
)

const (
	// DefaultExportDuration is the default data export duration.
	DefaultExportDuration = time.Hour * 24

	// DefaultExportSampleInterval is the default interval of exported metric samples.
	DefaultExportSampleInterval = time.Second * 15

	DefaultHost = "localhost"

	DefaultPort = 9089
)

// ExportConfig represents the configuration of the export command.
type ExportConfig struct {
	Host                    string
	Port                    uint16
	Duration         		time.Duration
	SampleInterval   		time.Duration
	MatcherSetExpressions 	[]string
}