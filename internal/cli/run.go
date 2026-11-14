package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tuan-nguyenduc/swapsafe/internal/generator"
	"github.com/tuan-nguyenduc/swapsafe/internal/prober"
	"github.com/tuan-nguyenduc/swapsafe/internal/report"
	api "github.com/tuan-nguyenduc/swapsafe/pkg/api/v1alpha1"
)

func newRunCommand() *cobra.Command {
	var (
		in      inputFlags
		opts    prober.Options
		outputs []string
		failOn  string
	)

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Probe both endpoints and report the differences",
		Long: "run sends the same requests to the ingress-nginx endpoint and to the Gateway\n" +
			"endpoint, compares the answers, and reports every difference that matters.\n\n" +
			"It connects to the endpoint addresses but presents each route's own hostname,\n" +
			"so a Gateway can be checked before DNS points at it. Nothing is written to any\n" +
			"cluster, and only GET, HEAD and OPTIONS are sent.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			threshold, err := api.ParseSeverity(failOn)
			if err != nil {
				return fmt.Errorf("--fail-on: %w", err)
			}
			for _, format := range outputs {
				if !slices.Contains(report.Formats(), format) {
					return fmt.Errorf("--output %q: want %s", format, strings.Join(report.Formats(), ", "))
				}
			}

			collected, err := in.collect(cmd.Context())
			if err != nil {
				return err
			}
			gen := generator.Generate(collected.Routes)

			res, err := probe(cmd.Context(), collected, gen, opts)
			if err != nil {
				return err
			}

			for _, format := range outputs {
				if err := report.Write(cmd.OutOrStdout(), format, res); err != nil {
					return err
				}
			}

			if n := exceeds(res.Findings, threshold); n > 0 {
				return exitError{
					code: ExitFindings,
					msg:  fmt.Sprintf("%d findings at or above %s", n, threshold),
				}
			}
			return nil
		},
	}

	in.register(cmd)
	f := cmd.Flags()
	f.StringVar(&opts.Baseline, "baseline", "", "ingress-nginx endpoint, e.g. https://203.0.113.10")
	f.StringVar(&opts.Candidate, "candidate", "", "Gateway endpoint, e.g. https://203.0.113.20")
	f.StringSliceVar(&outputs, "output", []string{report.FormatTable},
		"output format, repeatable: "+strings.Join(report.Formats(), ", "))
	f.StringVar(&failOn, "fail-on", string(api.SeverityCritical),
		"exit 2 when a finding reaches this severity: critical, major, minor, info")
	f.IntVar(&opts.Concurrency, "concurrency", prober.DefaultConcurrency, "requests in flight at once")
	f.Float64Var(&opts.RatePerSecond, "rate", prober.DefaultRatePerSecond, "requests per second, per endpoint")
	f.DurationVar(&opts.Timeout, "timeout", prober.DefaultTimeout, "per-request timeout")
	f.BoolVar(&opts.InsecureSkipVerify, "insecure-skip-verify", false, "do not verify TLS certificates")
	f.BoolVar(&opts.AllowUnsafeMethods, "allow-unsafe-methods", false,
		"send methods that can change state on a live system")
	f.BoolVar(&opts.IncludeBodies, "include-bodies", false,
		"keep up to 4 KiB of each response body in the report")

	_ = cmd.MarkFlagRequired("baseline")
	_ = cmd.MarkFlagRequired("candidate")

	return cmd
}
