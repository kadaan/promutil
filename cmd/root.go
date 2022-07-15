package cmd

import (
	goflag "flag"
	"fmt"
	"github.com/spf13/cobra"
	"k8s.io/klog/v2"
	"os"
	"time"
)

const (
	directoryKey       = "directory"
	outputDirectoryKey = "output-directory"
	startKey           = "start"
	endKey             = "end"
	sampleIntervalKey  = "sample-interval"
	matcherKey         = "matcher"
	schemeKey          = "scheme"
	hostKey            = "host"
	portKey            = "port"
	parallelismKey     = "parallelism"
	ruleConfigFileKey  = "rule-config-file"
	ruleGroupFilterKey = "rule-group-filter"
	ruleNameFilterKey  = "rule-name-filter"
)

var (
	end       = time.Now().UTC()
	start     = end.Add(-6 * time.Hour)
	verbosity int
	rootCmd   = &cobra.Command{
		Use:   "promutil",
		Short: "prometheus utilities",
		Long: `promutil provides a set of utilities for working with a Prometheus 
TSDB.  It allows data generation, recording rule backfilling, data 
migration, etc.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			klog.InitFlags(nil)
			return goflag.CommandLine.Parse([]string{
				"--skip_headers=true",
				fmt.Sprintf("-v=%d", verbosity),
			})
		},
	}
)

var osExit = os.Exit

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		klog.Errorln(err)
		osExit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().CountVarP(&verbosity, "verbose", "v", "enables verbose logging (multiple times increases verbosity)")
}
