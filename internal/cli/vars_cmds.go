// ars set, ars unset and ars vars save, remove and list the variables.

package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ynn3m/arsenal/internal/session"
	"github.com/ynn3m/arsenal/internal/tui"
)

const globalFlagHelp = "use global variables (every session) instead of the session's"

func newSetCmd() *cobra.Command {
	var global bool
	cmd := &cobra.Command{
		Use:   "set key=value [key=value ...]",
		Short: "Set variables used to fill commands",
		Long:  "Sets variables in the current session. Use -g to set them for every session.",
		Example: "  ars set target=10.10.10.5 domain=example.local\n" +
			"  ars set -g arsenal_prefix_cmd=\"proxychains -q\"   # for every session",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pairs := session.Vars{}
			var order []string
			for _, a := range args {
				k, v, err := session.ParsePair(a)
				if err != nil {
					return err
				}
				if _, dup := pairs[k]; !dup {
					order = append(order, k)
				}
				pairs[k] = v
			}
			if global {
				if err := session.SetGlobalVars(pairs); err != nil {
					return err
				}
			} else {
				sess, err := requireSession()
				if err != nil {
					return err
				}
				if err := session.SetVars(sess.Dir, pairs); err != nil {
					return err
				}
			}
			for _, k := range order {
				fmt.Fprintln(cmd.OutOrStdout(), tui.Success(fmt.Sprintf("%s = %s", tui.KeyStyle.Render(k), pairs[k])))
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&global, "global", "g", false, globalFlagHelp)
	return cmd
}

func newUnsetCmd() *cobra.Command {
	var global bool
	cmd := &cobra.Command{
		Use:   "unset key [key ...]",
		Short: "Remove variables",
		Long:  "Removes a variable you set. Use -g for a global one.",
		Example: "  ars unset domain\n" +
			"  ars unset -g arsenal_prefix_cmd",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, k := range args {
				if err := session.ValidateKey(k); err != nil {
					return err
				}
			}
			var missing []string
			if global {
				var err error
				if missing, err = session.UnsetGlobalVars(args); err != nil {
					return err
				}
			} else {
				sess, err := requireSession()
				if err != nil {
					return err
				}
				if missing, err = session.UnsetVars(sess.Dir, args); err != nil {
					return err
				}
			}
			isMissing := map[string]bool{}
			for _, k := range missing {
				isMissing[k] = true
			}
			for _, k := range args {
				if !isMissing[k] {
					fmt.Fprintln(cmd.OutOrStdout(), tui.Success("Removed "+tui.KeyStyle.Render(k)))
				}
			}
			if len(missing) > 0 {
				return fmt.Errorf("not set: %s", strings.Join(missing, ", "))
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&global, "global", "g", false, globalFlagHelp)
	return cmd
}

func newVarsCmd() *cobra.Command {
	var plain, onlyGlobal bool
	cmd := &cobra.Command{
		Use:   "vars",
		Short: "Show the variables in effect and where each comes from",
		Long:  "Lists every variable you set and where each value comes from.",
		Example: "  ars vars\n" +
			"  ars vars --plain   # key=value lines, for scripts",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			var r session.Resolved
			var err error
			title := "Variables"
			switch {
			case onlyGlobal:
				r, err = session.Resolve("")
				title = "Global variables"
			default:
				sess, ok, aerr := currentSession()
				if aerr != nil {
					return aerr
				}
				if ok {
					title = "Variables of " + sess.Name
				} else {
					title = "Global variables"
				}
				r, err = session.Resolve(sess.Dir)
			}
			if err != nil {
				return err
			}

			printVars(out, r, title, plain)
			return nil
		},
	}
	cmd.Flags().BoolVar(&plain, "plain", false, "print key=value lines, for scripts")
	cmd.Flags().BoolVarP(&onlyGlobal, "global", "g", false, "show only the global variables")
	return cmd
}

func printVars(out io.Writer, r session.Resolved, title string, plain bool) {
	keys := r.Vars.Keys()
	if plain {
		for _, k := range keys {
			fmt.Fprintf(out, "%s=%s\n", k, r.Vars[k])
		}
		return
	}
	fmt.Fprintln(out, tui.Heading(title))
	if len(keys) == 0 {
		fmt.Fprintln(out, tui.Muted("No variables set. Try ars set target=10.10.10.5"))
		return
	}
	width := 0
	for _, k := range keys {
		width = max(width, len(k))
	}
	for _, k := range keys {
		pad := strings.Repeat(" ", width-len(k))
		fmt.Fprintf(out, "  %s%s  %s  %s\n", tui.KeyStyle.Render(k), pad, r.Vars[k], tui.Muted("("+string(r.Source[k])+")"))
	}
}
