// Package cli wires the swapsafe command tree together.
package cli

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// NewRootCommand builds the swapsafe command tree.
func NewRootCommand() *cobra.Command {
	var logLevel, logFormat string

	cmd := &cobra.Command{
		Use:   "swapsafe",
		Short: "Verify that Gateway API reproduces ingress-nginx behaviour",
		Long: "swapsafe probes an ingress-nginx endpoint and a Gateway endpoint with the same\n" +
			"requests and reports every meaningful difference, attributed to the\n" +
			"ingress-nginx annotation that caused it. It is read-only: it never writes to a\n" +
			"cluster and never changes traffic.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			logger, err := newLogger(logLevel, logFormat)
			if err != nil {
				return err
			}
			slog.SetDefault(logger)
			return nil
		},
	}

	cmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "log level: debug, info, warn, error")
	cmd.PersistentFlags().StringVar(&logFormat, "log-format", "text", "log format: text or json")

	cmd.AddCommand(newScanCommand(), newRunCommand(), newVersionCommand())

	return cmd
}

// newLogger returns a slog logger writing to stderr, so that report output on
// stdout stays machine-readable.
func newLogger(level, format string) (*slog.Logger, error) {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		return nil, fmt.Errorf("invalid --log-level %q: %w", level, err)
	}

	opts := &slog.HandlerOptions{Level: lvl}
	switch strings.ToLower(format) {
	case "text":
		return slog.New(slog.NewTextHandler(os.Stderr, opts)), nil
	case "json":
		return slog.New(slog.NewJSONHandler(os.Stderr, opts)), nil
	default:
		return nil, fmt.Errorf("invalid --log-format %q: want text or json", format)
	}
}
