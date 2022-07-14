package cmd

import (
	"fmt"
	"github.com/ghodss/yaml"
	"github.com/kadaan/promutil/config"
	"github.com/kadaan/promutil/lib/block"
	"github.com/kadaan/promutil/lib/generator"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"io/ioutil"
	"os"
)

const (
	metricConfigFileKey = "metric-config-file"
)

var (
	generateConfig config.GenerateConfig

	// generateCmd represents the generate command
	generateCmd = &cobra.Command{
		Use:   "generate",
		Short: "Generate prometheus data",
		Long:  `Generate prometheus data based on the provided data definition.`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if !generateConfig.Start.Before(generateConfig.End) {
				return errors.New("start time is not before end time")
			}
			metricConfig, err := loadMetricConfig()
			if err != nil {
				return err
			}
			generateConfig.MetricConfig = metricConfig
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := generator.Generate(&generateConfig); err != nil {
				return errors.Wrap(err, "generation of data failed")
			}
			return nil
		},
	}
)

func init() {
	rootCmd.AddCommand(generateCmd)

	generateCmd.Flags().Var(config.NewTimeValue(&generateConfig.Start, start), startKey, `time to start generating from`)
	_ = viper.BindPFlag(startKey, generateCmd.Flags().Lookup(startKey))

	generateCmd.Flags().Var(config.NewTimeValue(&generateConfig.End, end), endKey, `time to generate up through`)
	_ = viper.BindPFlag(endKey, generateCmd.Flags().Lookup(endKey))

	generateCmd.Flags().StringVar(&generateConfig.OutputDirectory, outputDirectoryKey, config.DefaultDataDirectory, "output directory to write tsdb data")
	_ = generateCmd.MarkFlagDirname(outputDirectoryKey)
	_ = viper.BindPFlag(outputDirectoryKey, generateCmd.Flags().Lookup(outputDirectoryKey))

	generateCmd.Flags().DurationVar(&generateConfig.SampleInterval, sampleIntervalKey, config.DefaultSampleInterval, "interval at which samples will be generated")
	_ = viper.BindPFlag(sampleIntervalKey, generateCmd.Flags().Lookup(sampleIntervalKey))

	generateCmd.Flags().StringVar(&generateConfig.MetricConfigFile, metricConfigFileKey, config.DefaultMetricConfigFile, "config file defining the time series to create")
	_ = generateCmd.MarkFlagRequired(metricConfigFileKey)
	_ = generateCmd.MarkFlagFilename(metricConfigFileKey, config.YamlFileExtensions...)
	_ = viper.BindPFlag(metricConfigFileKey, generateCmd.Flags().Lookup(metricConfigFileKey))

	generateCmd.Flags().Var(config.NewRecordingRulesValue(&generateConfig.RuleConfig), ruleConfigFileKey, "config file defining the rules to evaluate")
	_ = generateCmd.MarkFlagFilename(ruleConfigFileKey, config.YamlFileExtensions...)
	_ = viper.BindPFlag(ruleConfigFileKey, generateCmd.Flags().Lookup(ruleConfigFileKey))

	generateCmd.Flags().Uint8Var(&generateConfig.Parallelism, parallelismKey, block.MaxParallelism, "parallelism for generation")
	_ = viper.BindPFlag(parallelismKey, generateCmd.Flags().Lookup(parallelismKey))
}

func loadMetricConfig() (*config.MetricConfig, error) {
	var metricConfig config.MetricConfig
	if _, err := os.Stat(generateConfig.MetricConfigFile); err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("could not find file %s", generateConfig.MetricConfigFile))
	}
	yamlFile, err := ioutil.ReadFile(generateConfig.MetricConfigFile)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("could not read file %s", generateConfig.MetricConfigFile))
	}
	err = yaml.Unmarshal(yamlFile, &metricConfig)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("could not parse file %s", generateConfig.MetricConfigFile))
	}
	return &metricConfig, nil
}
