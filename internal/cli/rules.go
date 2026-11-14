package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/tuan-nguyenduc/swapsafe/pkg/rule"
)

func newRulesCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "rules",
		Short: "List the rules and the annotations they verify",
		Long: "rules prints what swapsafe knows how to check. An annotation that is not\n" +
			"listed here is still reported when it appears, as a finding saying it was\n" +
			"not verified — swapsafe never passes over one in silence.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "RULE\tANNOTATIONS")
			for _, r := range rule.All() {
				annotations := strings.Join(r.Annotations(), "\n\t")
				if annotations == "" {
					annotations = "(every route)"
				}
				_, _ = fmt.Fprintf(tw, "%s\t%s\n", r.ID(), annotations)
			}
			if err := tw.Flush(); err != nil {
				return fmt.Errorf("writing table: %w", err)
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "\n%d rules.\n", len(rule.All()))
			return err
		},
	}
}
