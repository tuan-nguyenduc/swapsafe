package cli

import (
	"runtime"

	"github.com/spf13/cobra"
)

// Set via -ldflags at build time; see the Makefile.
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version, commit and build date",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := cmd.OutOrStdout().Write([]byte(
				"swapsafe " + version + "\n" +
					"  commit: " + commit + "\n" +
					"  built:  " + date + "\n" +
					"  go:     " + runtime.Version() + " " + runtime.GOOS + "/" + runtime.GOARCH + "\n",
			))
			return err
		},
	}
}
