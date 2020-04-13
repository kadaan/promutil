package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"os"
)

// completionCmd represents the completion command
var (
	completionShells = map[string]func(*cobra.Command) error {
		"bash": bashCompletion,
		"zsh": zshCompletion,
	}
	completionCmd = &cobra.Command{
		Use: "completion SHELL",
		DisableFlagsInUseLine: true,
		Short: "Output shell completion code for the specified shell (bash or zsh)",
		Long: `Output shell completion code for the specified shell (bash or zsh).
The shell code must be evaluated to provide interactive
completion of promutil commands.  This can be done by sourcing it from
the .bash_profile.
Note for zsh users: [1] zsh completions are only supported in versions of zsh >= 5.2`,
		Example: `# Installing bash completion on macOS using homebrew
## You need add the completion to your completion directory
	promutil completion bash > $(brew --prefix)/etc/bash_completion.d/promutil
# Installing bash completion on Linux
## If bash-completion is not installed on Linux, please install the 'bash-completion' package
## via your distribution's package manager.
## Load the promutil completion code for bash into the current shell
	source <(promutil completion bash)
## Write bash completion code to a file and source if from .bash_profile
	promutil completion bash > ~/.promutil/completion.bash.inc
	printf "
	  # promutil shell completion
	  source '$HOME/.promutil/completion.bash.inc'
	  " >> $HOME/.bash_profile
	source $HOME/.bash_profile
# Load the promutil completion code for zsh[1] into the current shell
	source <(promutil completion zsh)
# Set the promutil completion code for zsh[1] to autoload on startup
	promutil completion zsh > "${fpath[1]}/_promutil"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("Shell not specified.")
			}
			if len(args) > 1 {
				return fmt.Errorf("Too many arguments. Expected only the shell type.")
			}
			run, found := completionShells[args[0]]
			if !found {
				return fmt.Errorf("Unsupported shell type %q.", args[0])
			}
			return run(cmd)
		},
		ValidArgs: []string{"bash", "zsh"},
	}
)

func init() {
	rootCmd.AddCommand(completionCmd)
}

func bashCompletion(cmd *cobra.Command) error {
	return cmd.GenBashCompletion(os.Stdout)
}

func zshCompletion(cmd *cobra.Command) error {
	return  cmd.GenZshCompletion(os.Stdout)
}