package cmd

import (
	goflag "flag"
	"fmt"
	"github.com/spf13/cobra"
	"k8s.io/klog/v2"
	"os"
)

const (
	durationKey        = "duration"
	sampleIntervalKey  = "sample-interval"
)

var (
	verbosity int
	cfgFile string
    rootCmd = &cobra.Command{
		Use:   "promutil",
		Short: "prometheus utilities",
		Long: `promutil is a collection of utilities that work with prometheus.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			klog.InitFlags(nil)
			goflag.CommandLine.Parse([]string{
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