package cli

import (
	"slices"

	"github.com/spf13/cobra"

	"github.com/tuan-nguyenduc/swapsafe/internal/generator"
	"github.com/tuan-nguyenduc/swapsafe/internal/report"
	api "github.com/tuan-nguyenduc/swapsafe/pkg/api/v1alpha1"
	"github.com/tuan-nguyenduc/swapsafe/pkg/rule"
)

func newScanCommand() *cobra.Command {
	var in inputFlags

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "List the routes and the rules that would probe them",
		Long: "scan reads the Ingress and HTTPRoute objects, pairs them, and prints what a\n" +
			"run would check. It sends no requests, so it is safe to point at anything.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			collected, err := in.collect(cmd.Context())
			if err != nil {
				return err
			}
			gen := generator.Generate(collected.Routes)

			findings := slices.Concat(collected.Findings, gen.Findings)
			return report.ScanTable(cmd.OutOrStdout(), collected.Routes, rulesFor, findings)
		},
	}

	in.register(cmd)
	return cmd
}

// rulesFor names the rules that will probe a route, for the scan table.
func rulesFor(route api.RouteSpec) []string {
	var ids []string
	for _, r := range rule.For(route) {
		ids = append(ids, r.ID())
	}
	return ids
}
