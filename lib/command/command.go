package command

import (
	goflag "flag"
	"fmt"
	"github.com/kadaan/promutil/config"
	"github.com/kadaan/promutil/lib/errors"
	"github.com/kadaan/promutil/version"
	"github.com/kadaan/tracerr"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"k8s.io/klog/v2"
	"log"
	"os"
)

var (
	osExit = os.Exit
)

type RootCommand interface {
	Execute()
	addCommand(cmd *cobra.Command)
}

func NewRootCommand(short string, long string) RootCommand {
	log.SetFlags(0)
	r := new(rootCommand)
	r.cmd = &cobra.Command{
		Use:   version.Name,
		Short: short,
		Long:  long,
	}
	r.cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		klog.InitFlags(nil)
		return goflag.CommandLine.Parse([]string{
			"--skip_headers=true",
			fmt.Sprintf("-v=%d", r.verbosity),
		})
	}
	r.addVersionCommand(r.cmd)
	r.addCompletionCommand(r.cmd)
	cobra.OnInitialize(r.initConfig)
	r.cmd.PersistentFlags().CountVarP(&r.verbosity, "verbose", "v", "enables verbose logging (multiple times increases verbosity)")
	r.cmd.PersistentFlags().StringVar(&r.cfgFile, "config", "", "config file (default is ."+version.Name+".config)")
	return r
}

type rootCommand struct {
	verbosity int
	cfgFile   string
	cmd       *cobra.Command
}

func (r *rootCommand) addCommand(cmd *cobra.Command) {
	r.cmd.AddCommand(cmd)
}

func (r *rootCommand) addVersionCommand(cmd *cobra.Command) {
	cmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Prints the " + version.Name + " version",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Printf(version.Print())
		},
	})
}

func (r *rootCommand) addCompletionCommand(cmd *cobra.Command) {
	completionShells := map[string]func(*cobra.Command) error{
		"bash": func(c *cobra.Command) error {
			return cmd.GenBashCompletion(os.Stdout)
		},
		"zsh": func(c *cobra.Command) error {
			return cmd.GenZshCompletion(os.Stdout)
		},
	}
	completionCommand := &cobra.Command{
		Use:                   "completion SHELL",
		DisableFlagsInUseLine: true,
		Short:                 "Output shell completion code for the specified shell (bash or zsh)",
		Long: `Output shell completion code for the specified shell (bash or zsh).
The shell code must be evaluated to provide interactive
completion of ` + version.Name + ` commands.  This can be done by sourcing it from
the .bash_profile.
Note for zsh users: [1] zsh completions are only supported in versions of zsh >= 5.2`,
		Example: `# Installing bash completion on macOS using homebrew
## You need add the completion to your completion directory
	` + version.Name + ` completion bash > $(brew --prefix)/etc/bash_completion.d/` + version.Name + `
# Installing bash completion on Linux
## If bash-completion is not installed on Linux, please install the 'bash-completion' package
## via your distribution's package manager.
## Load the ` + version.Name + ` completion code for bash into the current shell
	source <(` + version.Name + ` completion bash)
## Write bash completion code to a file and source if from .bash_profile
	` + version.Name + ` completion bash > ~/.` + version.Name + `/completion.bash.inc
	printf "
	  # ` + version.Name + ` shell completion
	  source '$HOME/.` + version.Name + `/completion.bash.inc'
	  " >> $HOME/.bash_profile
	source $HOME/.bash_profile
# Load the ` + version.Name + ` completion code for zsh[1] into the current shell
	source <(` + version.Name + ` completion zsh)
# Set the ` + version.Name + ` completion code for zsh[1] to autoload on startup
	` + version.Name + ` completion zsh > "${fpath[1]}/_` + version.Name + `"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return errors.New("shell not specified")
			}
			if len(args) > 1 {
				return errors.New("too many arguments, expected only the shell type")
			}
			run, found := completionShells[args[0]]
			if !found {
				return errors.Errorf("unsupported shell type %q", args[0])
			}
			return run(cmd)
		},
		ValidArgs: []string{"bash", "zsh"},
	}
	cmd.AddCommand(completionCommand)
}

func (r *rootCommand) initConfig() {
	if r.cfgFile != "" {
		viper.SetConfigFile(r.cfgFile)
	} else {
		workingDir, err := os.Getwd()
		if err != nil {
			tracerr.PrintSourceColor(err)
			osExit(1)
		}

		viper.AddConfigPath(workingDir)
		viper.SetConfigName("." + version.Name)
	}

	viper.AutomaticEnv()
	if err := viper.ReadInConfig(); err == nil {
		klog.V(0).Infoln("Using config file:", viper.ConfigFileUsed())
	}

	r.postInitCommands(r.cmd.Commands())
}

func (r *rootCommand) postInitCommands(commands []*cobra.Command) {
	for _, c := range commands {
		r.presetRequiredFlags(c)
		if c.HasSubCommands() {
			r.postInitCommands(c.Commands())
		}
	}
}

func (r *rootCommand) presetRequiredFlags(cmd *cobra.Command) {
	_ = viper.BindPFlags(cmd.Flags())
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if viper.IsSet(f.Name) && viper.GetString(f.Name) != "" {
			_ = cmd.Flags().Set(f.Name, viper.GetString(f.Name))
		}
	})
}

func (r *rootCommand) Execute() {
	if err := r.cmd.Execute(); err != nil {
		tracerr.PrintSourceColor(err)
		osExit(1)
	}
}

type Command[C any] interface {
	Configure(func(fb config.FlagBuilder, cfg *C))
}

type command[C any] struct {
	cfg *C
	fb  config.FlagBuilder
}

func (c *command[C]) Configure(f func(fb config.FlagBuilder, cfg *C)) {
	f(c.fb, c.cfg)
}

func NewCommand[C any](root RootCommand, use string, short string, long string, cfg *C, task Task[C]) Command[C] {
	c := &cobra.Command{
		Use:   use,
		Short: short,
		Long:  long,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := task.Run(cfg); err != nil {
				return errors.Wrap(err, "%s failed", use)
			}
			return nil
		},
	}
	root.addCommand(c)
	return &command[C]{
		cfg: cfg,
		fb:  config.NewFlagBuilder(c),
	}
}

type Task[C any] interface {
	Run(cfg *C) error
}
