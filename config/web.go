package config

import (
	"net/url"
	"time"
)

// WebConfig represents the configuration of the web command.
type WebConfig struct {
	ListenAddress  ListenAddress
	Host           *url.URL
	SampleInterval time.Duration
	Parallelism    uint8
}
