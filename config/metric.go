package config

import (
	"fmt"
	"github.com/ghodss/yaml"
	"github.com/kadaan/promutil/lib/errors"
	"io/ioutil"
	"os"
)

type TimeSeries []struct {
	Name       string              `yaml:"name"`
	Instances  []string            `yaml:"instances"`
	Labels     []map[string]string `yaml:"labels"`
	Expression string              `yaml:"expression"`
}

type MetricConfig struct {
	TimeSeries TimeSeries `yaml:"timeSeries"`
}

type metricConfigValue MetricConfig

func NewMetricConfigValue(p *MetricConfig) *metricConfigValue {
	*p = MetricConfig{}
	return (*metricConfigValue)(p)
}

// String is used both by fmt.Print and by Cobra in help text
func (e *metricConfigValue) String() string {
	if len((*MetricConfig)(e).TimeSeries) == 0 {
		return "Empty"
	}
	return fmt.Sprintf("%d time series", len((*MetricConfig)(e).TimeSeries))
}

// Set must have pointer receiver, so it doesn't change the value of a copy
func (e *metricConfigValue) Set(v string) error {
	var metricConfig MetricConfig
	if _, err := os.Stat(v); err != nil {
		return errors.Wrap(err, "could not find file %s", v)
	}
	yamlFile, err := ioutil.ReadFile(v)
	if err != nil {
		return errors.Wrap(err, "could not read file %s", v)
	}
	err = yaml.Unmarshal(yamlFile, &metricConfig)
	if err != nil {
		return errors.Wrap(err, "could not parse file %s", v)
	}
	*e = metricConfigValue(metricConfig)
	return nil
}

// Type is only used in help text
func (e *metricConfigValue) Type() string {
	return "metricConfig"
}
