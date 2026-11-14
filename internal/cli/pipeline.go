package cli

import (
	"context"
	"fmt"
	"slices"

	"github.com/spf13/cobra"

	"github.com/tuan-nguyenduc/swapsafe/internal/attribution"
	"github.com/tuan-nguyenduc/swapsafe/internal/collector"
	"github.com/tuan-nguyenduc/swapsafe/internal/differ"
	"github.com/tuan-nguyenduc/swapsafe/internal/generator"
	"github.com/tuan-nguyenduc/swapsafe/internal/normalize"
	"github.com/tuan-nguyenduc/swapsafe/internal/prober"
	api "github.com/tuan-nguyenduc/swapsafe/pkg/api/v1alpha1"
)

// inputFlags are the sources of Ingress and HTTPRoute objects, shared by scan
// and run. Reading from a cluster is M1-T3 and is not wired up yet.
type inputFlags struct {
	ingressFile   string
	httpRouteFile string
}

func (in *inputFlags) register(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringVar(&in.ingressFile, "ingress-file", "", "YAML manifest holding the Ingress objects")
	f.StringVar(&in.httpRouteFile, "httproute-file", "", "YAML manifest holding the HTTPRoute objects")
}

func (in *inputFlags) collect(ctx context.Context) (collector.Result, error) {
	if in.ingressFile == "" && in.httpRouteFile == "" {
		return collector.Result{}, fmt.Errorf("nothing to read: pass --ingress-file and --httproute-file")
	}
	return collector.FromFiles(ctx, in.ingressFile, in.httpRouteFile)
}

// probe runs the half of the pipeline that touches the network and returns a
// finished results document.
func probe(ctx context.Context, collected collector.Result, gen generator.Result, opts prober.Options) (api.Results, error) {
	p, err := prober.New(opts)
	if err != nil {
		return api.Results{}, err
	}
	defer p.Close()

	observations, err := p.Run(ctx, gen.TestCases)
	if err != nil {
		return api.Results{}, fmt.Errorf("probing: %w", err)
	}

	byID := make(map[string]api.TestCase, len(gen.TestCases))
	for _, tc := range gen.TestCases {
		byID[tc.ID] = tc
	}
	for id, list := range observations {
		for i, o := range list {
			list[i] = normalize.Observation(o, byID[id].Request, normalize.Options{})
		}
	}

	// slices.Concat, not append: appending onto collected.Findings would
	// write into the collector's own backing array.
	findings := slices.Concat(collected.Findings, gen.Findings,
		differ.DiffAll(gen.TestCases, observations))

	res := api.NewResults(api.Metadata{
		Version:   version,
		Baseline:  opts.Baseline,
		Candidate: opts.Candidate,
	})
	res.Routes = collected.Routes
	res.TestCases = gen.TestCases
	res.Observations = observations
	res.Findings = attribution.Apply(findings, collected.Routes, gen.TestCases)
	return res, nil
}

// worstSeverity returns the rank of the most severe finding.
func exceeds(findings []api.Finding, threshold api.Severity) int {
	var n int
	for _, f := range findings {
		if f.Severity.AtLeast(threshold) {
			n++
		}
	}
	return n
}
