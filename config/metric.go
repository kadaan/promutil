package config

type MetricConfig struct {
	TimeSeries   []struct {
		Name       string `yaml:"name"`
		Instances  []string `yaml:"instances"`
		Labels     []map[string]string `yaml:"labels"`
		Expression string `yaml:"expression"`
	} `yaml:"timeSeries"`
}