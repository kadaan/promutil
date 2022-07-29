package config

import (
	"github.com/kadaan/promutil/lib/errors"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"net/url"
	"regexp"
	"time"
)

const (
	directoryKey          = "directory"
	outputDirectoryKey    = "output-directory"
	startKey              = "start"
	endKey                = "end"
	sampleIntervalKey     = "sample-interval"
	parallelismKey        = "parallelism"
	ruleConfigFileKey     = "rule-config-file"
	ruleGroupFilterKey    = "rule-group-filter"
	ruleNameFilterKey     = "rule-name-filter"
	metricConfigFileKey   = "metric-config-file"
	hostKey               = "host"
	matcherKey            = "matcher"
	listenAddressKey      = "listenAddress"
	defaultSampleInterval = time.Second * 15
	defaultDataDirectory  = "data/"
)

var (
	defaultEnd              = time.Now().UTC()
	defaultStart            = defaultEnd.Add(-6 * time.Hour)
	defaultHost             = MustParseUrl("http://localhost:9090")
	defaultRuleGroupFilters = []*regexp.Regexp{regexp.MustCompile(".+")}
	defaultRuleNameFilters  = []*regexp.Regexp{regexp.MustCompile(".+")}
	yamlFileExtensions      = []string{"yml", "yaml"}
)

func NewFlagBuilder(cmd *cobra.Command) FlagBuilder {
	return &flagBuilder{
		cmd: cmd,
	}
}

type Flag interface {
	Required() Flag
}

type FileFlag interface {
	Flag
	Extensions(extensions ...string) FileFlag
}

type compositeFlag struct {
	flags []Flag
}

func (f *compositeFlag) Required() Flag {
	for _, c := range f.flags {
		_ = c.Required()
	}
	return f
}

type flag struct {
	builder *flagBuilder
	flag    *pflag.Flag
}

func (f *flag) Required() Flag {
	_ = f.builder.cmd.MarkFlagRequired(f.flag.Name)
	return f
}

func (f *flag) Extensions(extensions ...string) FileFlag {
	_ = f.builder.cmd.MarkFlagFilename(f.flag.Name, extensions...)
	return f
}

type FlagBuilder interface {
	TimeRange(startDest *time.Time, endDest *time.Time, usage string) Flag
	StartTime(dest *time.Time, usage string) Flag
	EndTime(dest *time.Time, usage string) Flag
	Time(dest *time.Time, name string, defaultValue time.Time, usage string) Flag
	OutputDirectory(dest *string, usage string) Flag
	Directory(dest *string, usage string) Flag
	MetricConfig(dest *MetricConfig, usage string) FileFlag
	File(dest *string, name string, defaultValue string, usage string) FileFlag
	SampleInterval(dest *time.Duration, usage string) Flag
	Duration(dest *time.Duration, name string, defaultValue time.Duration, usage string) Flag
	RecordingRules(dest *RecordingRules, usage string) Flag
	Parallelism(dest *uint8, defaultValue uint8, usage string) Flag
	Regex(dest *[]*regexp.Regexp, name string, defaultValue []*regexp.Regexp, usage string) Flag
	RuleGroupFilters(dest *[]*regexp.Regexp, usage string) Flag
	RuleNameFilters(dest *[]*regexp.Regexp, usage string) Flag
	URL(dest **url.URL, name string, defaultValue *url.URL, usage string) Flag
	Host(dest **url.URL, usage string) Flag
	Matchers(dest *map[string][]*labels.Matcher, usage string) Flag
	ListenAddress(dest *ListenAddress, usage string) Flag
}

type flagBuilder struct {
	cmd *cobra.Command
}

func (fb *flagBuilder) newFlag(name string, creator func(flagSet *pflag.FlagSet)) *flag {
	creator(fb.cmd.Flags())
	f := fb.cmd.Flags().Lookup(name)
	_ = viper.BindPFlag(name, f)
	return &flag{
		builder: fb,
		flag:    f,
	}
}

func (fb *flagBuilder) addValidation(validation func(cmd *cobra.Command, args []string) error) {
	if fb.cmd.PreRunE != nil {
		existingValidation := fb.cmd.PreRunE
		fb.cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
			if err := validation(cmd, args); err != nil {
				return err
			}
			return existingValidation(cmd, args)
		}
	} else {
		fb.cmd.PreRunE = validation
	}
}

func (fb *flagBuilder) TimeRange(startDest *time.Time, endDest *time.Time, usage string) Flag {
	startFlag := fb.Time(startDest, startKey, defaultStart, usage+" from")
	endFlag := fb.Time(endDest, endKey, defaultEnd, usage+" to")
	fb.addValidation(func(cmd *cobra.Command, args []string) error {
		if !(*startDest).Before(*endDest) {
			return errors.New("start time is not before end time")
		}
		return nil
	})
	return &compositeFlag{
		flags: []Flag{startFlag, endFlag},
	}
}

func (fb *flagBuilder) StartTime(dest *time.Time, usage string) Flag {
	return fb.Time(dest, startKey, defaultStart, usage)
}

func (fb *flagBuilder) EndTime(dest *time.Time, usage string) Flag {
	return fb.Time(dest, endKey, defaultEnd, usage)
}

func (fb *flagBuilder) Time(dest *time.Time, name string, defaultValue time.Time, usage string) Flag {
	return fb.newFlag(name, func(flagSet *pflag.FlagSet) {
		flagSet.Var(NewTimeValue(dest, defaultValue), name, usage)
	})
}

func (fb *flagBuilder) OutputDirectory(dest *string, usage string) Flag {
	return fb.directory(dest, outputDirectoryKey, defaultDataDirectory, usage)
}

func (fb *flagBuilder) Directory(dest *string, usage string) Flag {
	return fb.directory(dest, directoryKey, defaultDataDirectory, usage)
}

func (fb *flagBuilder) directory(dest *string, name string, defaultValue string, usage string) Flag {
	return fb.newFlag(name, func(flagSet *pflag.FlagSet) {
		flagSet.StringVar(dest, name, defaultValue, usage)
		_ = fb.cmd.MarkFlagDirname(name)
	})
}

func (fb *flagBuilder) File(dest *string, name string, defaultValue string, usage string) FileFlag {
	return fb.newFlag(name, func(flagSet *pflag.FlagSet) {
		flagSet.StringVar(dest, name, defaultValue, usage)
		_ = fb.cmd.MarkFlagFilename(name)
	})
}

func (fb *flagBuilder) SampleInterval(dest *time.Duration, usage string) Flag {
	return fb.Duration(dest, sampleIntervalKey, defaultSampleInterval, usage)
}

func (fb *flagBuilder) Duration(dest *time.Duration, name string, defaultValue time.Duration, usage string) Flag {
	return fb.newFlag(name, func(flagSet *pflag.FlagSet) {
		flagSet.DurationVar(dest, name, defaultValue, usage)
	})
}

func (fb *flagBuilder) MetricConfig(dest *MetricConfig, usage string) FileFlag {
	return fb.newFlag(metricConfigFileKey, func(flagSet *pflag.FlagSet) {
		flagSet.Var(NewMetricConfigValue(dest), metricConfigFileKey, usage)
		_ = fb.cmd.MarkFlagFilename(metricConfigFileKey, yamlFileExtensions...)
	})
}

func (fb *flagBuilder) RecordingRules(dest *RecordingRules, usage string) Flag {
	return fb.newFlag(ruleConfigFileKey, func(flagSet *pflag.FlagSet) {
		flagSet.Var(NewRecordingRulesValue(dest), ruleConfigFileKey, usage)
		_ = fb.cmd.MarkFlagFilename(ruleConfigFileKey, yamlFileExtensions...)
	})
}

func (fb *flagBuilder) Parallelism(dest *uint8, defaultValue uint8, usage string) Flag {
	return fb.newFlag(parallelismKey, func(flagSet *pflag.FlagSet) {
		flagSet.Uint8Var(dest, parallelismKey, defaultValue, usage)
	})
}

func (fb *flagBuilder) Regex(dest *[]*regexp.Regexp, name string, defaultValue []*regexp.Regexp, usage string) Flag {
	return fb.newFlag(name, func(flagSet *pflag.FlagSet) {
		flagSet.Var(NewRegexValue(dest, defaultValue), name, usage)
	})
}

func (fb *flagBuilder) RuleGroupFilters(dest *[]*regexp.Regexp, usage string) Flag {
	return fb.Regex(dest, ruleGroupFilterKey, defaultRuleGroupFilters, usage)
}

func (fb *flagBuilder) RuleNameFilters(dest *[]*regexp.Regexp, usage string) Flag {
	return fb.Regex(dest, ruleNameFilterKey, defaultRuleNameFilters, usage)
}

func (fb *flagBuilder) URL(dest **url.URL, name string, defaultValue *url.URL, usage string) Flag {
	return fb.newFlag(name, func(flagSet *pflag.FlagSet) {
		flagSet.Var(NewUrlValue(dest, defaultValue), name, usage)
	})
}

func (fb *flagBuilder) Host(dest **url.URL, usage string) Flag {
	return fb.URL(dest, hostKey, defaultHost, usage)
}

func (fb *flagBuilder) Matchers(dest *map[string][]*labels.Matcher, usage string) Flag {
	return fb.newFlag(matcherKey, func(flagSet *pflag.FlagSet) {
		flagSet.Var(NewMatchersValue(dest), matcherKey, usage)
	})
}

func (fb *flagBuilder) ListenAddress(dest *ListenAddress, usage string) Flag {
	return fb.newFlag(listenAddressKey, func(flagSet *pflag.FlagSet) {
		flagSet.Var(NewListenAddressValue(dest, ListenAddress{Host: "", Port: 8080}), listenAddressKey, usage)
	})
}
