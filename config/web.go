package config

import (
	"net/url"
	"time"
)

const (
	DefaultListenAddress = ":8080"
)

// WebConfig represents the configuration of the web command.
type WebConfig struct {
	ListenAddress  ListenAddress
	Host           *url.URL
	SampleInterval time.Duration
}
