package config

import (
	"regexp"
	"time"
)

const (
	DefaultSampleInterval = time.Second * 15

	DefaultHost = "localhost"

	DefaultPort = 9089

	DefaultScheme = Http

	DefaultDataDirectory = "data/"
)

var (
	DefaultMatcher          []string
	DefaultRuleGroupFilters = []*regexp.Regexp{regexp.MustCompile(".+")}
	DefaultRuleNameFilters  = []*regexp.Regexp{regexp.MustCompile(".+")}
	YamlFileExtensions      = []string{"yml", "yaml"}
)
