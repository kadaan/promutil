package config

import (
	"github.com/prometheus/prometheus/model/labels"
	"net/url"
	"time"
)

// MigrateConfig represents the configuration of the migrate command.
type MigrateConfig struct {
	Host           *url.URL
	Start          time.Time
	End            time.Time
	SampleInterval time.Duration
	Matchers       map[string][]*labels.Matcher
	Directory      string
	Parallelism    uint8
}
