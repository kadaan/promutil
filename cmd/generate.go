package cmd

import (
	"fmt"
	"github.com/ghodss/yaml"
	"github.com/kadaan/promutil/config"
	"github.com/kadaan/promutil/lib"
	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/rulefmt"
	"github.com/prometheus/prometheus/promql/parser"
	errors2 "github.com/prometheus/tsdb/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"io/ioutil"
	"os"
)

const (
	outputDirectoryKey  = "output-directory"
	blockLengthKey      = "block-length"
	metricConfigFileKey = "metric-config-file"
	ruleConfigFileKey   = "rule-config-file"
)

var (
	generateConfig config.GenerateConfig

	// generateCmd represents the generate command
	generateCmd = &cobra.Command{
		Use:   "generate",
		Short: "Generate prometheus data",
		Long: `Generate prometheus data based on the provided data definition.`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			metricConfig, err := loadMetricConfig()
			if err != nil {
				return err
			}
			generateConfig.MetricConfig = metricConfig
			rulesConfig, err := loadRecordingRulesConfig()
			if err != nil {
				return err
			}
			generateConfig.RuleConfig = rulesConfig
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			generator, err := lib.NewGenerator(&generateConfig)
			if err != nil {
				return errors.Wrap(err, "generation of TSDB data failed")
			}
			return generator.Generate()
		},
	}
)

func init() {
	rootCmd.AddCommand(generateCmd)

	generateCmd.Flags().DurationVar(&generateConfig.Duration, durationKey, config.DefaultGenerateDuration, "duration of data to generate")
	viper.BindPFlag(durationKey, generateCmd.Flags().Lookup(durationKey))

	generateCmd.Flags().StringVar(&generateConfig.OutputDirectory, outputDirectoryKey, config.DefaultGenerateOutputDirectory, "output directory to generate data in")
	generateCmd.MarkFlagDirname(outputDirectoryKey)
	viper.BindPFlag(outputDirectoryKey, generateCmd.Flags().Lookup(outputDirectoryKey))

	generateCmd.Flags().DurationVar(&generateConfig.SampleInterval, sampleIntervalKey, config.DefaultGenerateSampleInterval, "interval at which samples will be generated")
	viper.BindPFlag(sampleIntervalKey, generateCmd.Flags().Lookup(sampleIntervalKey))

	generateCmd.Flags().DurationVar(&generateConfig.BlockLength, blockLengthKey, config.DefaultGenerateBlockLength, "generated block length")
	viper.BindPFlag(blockLengthKey, generateCmd.Flags().Lookup(blockLengthKey))

	generateCmd.Flags().StringVar(&generateConfig.MetricConfigFile, metricConfigFileKey, "", "Config file defining the time series to create")
	generateCmd.MarkFlagRequired(metricConfigFileKey)
	generateCmd.MarkFlagFilename(metricConfigFileKey, "yml", "yaml")
	viper.BindPFlag(metricConfigFileKey, generateCmd.Flags().Lookup(metricConfigFileKey))

	generateCmd.Flags().StringArrayVar(&generateConfig.RuleConfigFiles, ruleConfigFileKey, []string{}, "Config file defining the rules to evaluate")
	generateCmd.MarkFlagFilename(ruleConfigFileKey, "yml", "yaml")
	viper.BindPFlag(ruleConfigFileKey, generateCmd.Flags().Lookup(ruleConfigFileKey))
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

func loadRecordingRulesConfig() (config.RecordingRules, error) {
	var rules config.RecordingRules
	for _, recordingRulesFile := range generateConfig.RuleConfigFiles {
		rgs, errs := rulefmt.ParseFile(recordingRulesFile)
		if errs != nil {
			multiError := errors2.MultiError{}
			for _, err := range errs {
				multiError.Add(err)
			}
			return nil, errors.Wrap(multiError, fmt.Sprintf("failed to parse recording rule file '%s'", recordingRulesFile))
		}
		for _, rg := range rgs.Groups {
			for _, rule := range rg.Rules {
				if rule.Record.Value !=  "" {
					expr, err := parser.ParseExpr(rule.Expr.Value)
					if err != nil {
						return nil, errors.Wrap(err, fmt.Sprintf("failed to parse recording rule expression '%s'", rule.Expr.Value))
					}
					recordingRule := &config.RecordingRule {
						Name: rule.Record.Value,
						Expression: expr,
						Labels: labels.FromMap(rule.Labels),
					}
					rules = append(rules, recordingRule)
				}
			}
		}
	}
	return rules, nil
}