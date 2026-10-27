// Command buildstamp reads the Go build information of binaries — wasm
// included, which `go version -m` cannot — and fails one built from a
// modified tree.
//
//	buildstamp check [--revision SHA] FILE...   exit 1 if any is not clean
//	buildstamp show FILE...                       print the build information
//
// Exit status: 0 clean, 1 a file is not clean (modified tree, no VCS stamp,
// another revision), 2 a file could not be read.
package main

import (
	"errors"
	"fmt"
	"os"
	"runtime/debug"

	"github.com/spf13/cobra"

	"github.com/go-fleettools/buildstamp"
)

// osExit is replaced by tests.
var osExit = os.Exit

// exitError carries the exit status.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string { return e.err.Error() }

// RootCmd is the command tree.
func RootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "buildstamp",
		Short:         "Read the Go build information of any binary, wasm included, and fail one built from a modified tree",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	var revision string
	check := &cobra.Command{
		Use:   "check [--revision SHA] FILE...",
		Short: "Fail unless each binary was built from a clean commit (vcs.modified=false)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var bad error
			for _, f := range args {
				bi, err := buildstamp.ReadFile(f)
				if err != nil {
					return &exitError{2, fmt.Errorf("%s: %w", f, err)}
				}
				if err := buildstamp.Clean(bi, revision); err != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "%s: NOT CLEAN: %v\n", f, err)
					bad = errors.Join(bad, fmt.Errorf("%s: %w", f, err))
					continue
				}
				rev, _ := buildstamp.Setting(bi, "vcs.revision")
				fmt.Fprintf(cmd.OutOrStdout(), "%s: clean: %s at %s\n", f, bi.Main.Version, rev)
			}
			if bad != nil {
				return &exitError{1, bad}
			}
			return nil
		},
	}
	check.Flags().StringVarP(&revision, "revision", "r", "", "also require this vcs.revision (a full commit hash)")
	show := &cobra.Command{
		Use:   "show FILE...",
		Short: "Print each binary's build information (as `go version -m` would, without the Go version)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, f := range args {
				bi, err := buildstamp.ReadFile(f)
				if err != nil {
					return &exitError{2, fmt.Errorf("%s: %w", f, err)}
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s:\n%s", f, indent(bi))
			}
			return nil
		},
	}
	root.AddCommand(check, show)
	return root
}

func indent(bi *debug.BuildInfo) string {
	s := ""
	line := ""
	for _, c := range bi.String() {
		line += string(c)
		if c == '\n' {
			s += "\t" + line
			line = ""
		}
	}
	return s
}

// run executes the command tree and returns the exit status.
func run(args []string) int {
	cmd := RootCmd()
	cmd.SetArgs(args)
	err := cmd.Execute()
	if err == nil {
		return 0
	}
	fmt.Fprintln(os.Stderr, "⛔", err)
	var ee *exitError
	if errors.As(err, &ee) {
		return ee.code
	}
	return 2
}

func main() { osExit(run(os.Args[1:])) }
