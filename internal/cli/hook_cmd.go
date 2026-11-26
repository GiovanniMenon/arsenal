// ars hook prints the shell function that lets ars change folder and fill the prompt.

package cli

import (
	_ "embed"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/ynn3m/arsenal/internal/setup"
	"github.com/ynn3m/arsenal/internal/tui"
)

const EnvCDFile = "ARSENAL_CD_FILE"

var (
	//go:embed shell/func.sh
	shellFunc string
	//go:embed shell/zsh.sh
	zshHooks string
	//go:embed shell/bash.sh
	bashHooks string
)

func hookScript(shell string) (string, error) {
	var hooks string
	switch shell {
	case "zsh":
		hooks = zshHooks
	case "bash":
		hooks = bashHooks
	default:
		return "", fmt.Errorf("unsupported shell %q: use zsh or bash", shell)
	}
	return shellFunc + hooks, nil
}

func newHookCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "hook <zsh|bash>",
		Hidden:    true,
		Short:     "Print the shell integration for zsh or bash",
		Long:      "Prints the shell code that lets ars change your directory, put commands in your prompt and log what you run.\nars setup adds it to your shell startup file for you.",
		Example:   "  ars hook zsh\n  ars hook bash",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"zsh", "bash"},
		RunE: func(cmd *cobra.Command, args []string) error {
			script, err := hookScript(args[0])
			if err != nil {
				return err
			}
			_, err = io.WriteString(cmd.OutOrStdout(), script)
			return err
		},
	}
}

func requestCD(dir string) error {
	f := os.Getenv(EnvCDFile)
	if f == "" {
		return nil
	}
	if err := os.WriteFile(f, []byte(dir), 0o600); err != nil {
		return fmt.Errorf("cannot tell your shell to change folder: %w", err)
	}
	return nil
}

func cdHint(dir string) string {
	if os.Getenv(EnvCDFile) != "" {
		return tui.Muted("  " + dir)
	}
	return tui.Muted("  " + dir + "\n  run ars setup to cd into sessions automatically")
}

func rcHasIntegration() bool {
	shell := setup.DetectShell(os.Getenv("SHELL"))
	home, err := os.UserHomeDir()
	if shell == "" || err != nil {
		return false
	}
	content, err := setup.ReadRC(setup.RCFile(shell, home, os.Getenv("ZDOTDIR")))
	return err == nil && setup.AnalyzeRC(content, shell).Current
}

const restartHint = "restart the terminal to enable shell integration"
