// Package collector reads Ingress and HTTPRoute objects and pairs them into
// the routes the rest of the pipeline works on. It never writes to a cluster.
package collector

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	networkingv1 "k8s.io/api/networking/v1"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/yaml"
)

// manifests holds the objects read from one or more YAML streams.
type manifests struct {
	ingresses  []networkingv1.Ingress
	httpRoutes []gatewayv1.HTTPRoute
}

// typeMeta is the minimum needed to decide what a document is before decoding it.
type typeMeta struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
}

// readFiles decodes every document of every file. Paths that resolve to the
// same file are read once, so passing one combined manifest as both
// --ingress-file and --httproute-file does not duplicate its objects.
func readFiles(ctx context.Context, paths ...string) (manifests, error) {
	var out manifests
	seen := make(map[string]bool, len(paths))

	for _, path := range paths {
		if path == "" {
			continue
		}
		key := path
		if abs, err := filepath.Abs(path); err == nil {
			key = abs
		}
		if seen[key] {
			continue
		}
		seen[key] = true

		f, err := os.Open(path) // #nosec G304 -- the path comes from the user's own flags
		if err != nil {
			return manifests{}, fmt.Errorf("open %s: %w", path, err)
		}
		err = decodeInto(ctx, f, &out)
		cerr := f.Close()
		if err != nil {
			return manifests{}, fmt.Errorf("read %s: %w", path, err)
		}
		if cerr != nil {
			return manifests{}, fmt.Errorf("close %s: %w", path, cerr)
		}
	}
	return out, nil
}

// decodeInto reads a multi-document YAML stream, keeping the Ingress and
// HTTPRoute documents and ignoring every other kind: users routinely point
// swapsafe at a manifest that also contains Services, Secrets and Gateways.
func decodeInto(ctx context.Context, r io.Reader, out *manifests) error {
	reader := k8syaml.NewYAMLReader(bufio.NewReader(r))

	for i := 0; ; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		doc, err := reader.Read()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("document %d: %w", i, err)
		}

		var meta typeMeta
		if err := yaml.Unmarshal(doc, &meta); err != nil {
			return fmt.Errorf("document %d: %w", i, err)
		}

		switch {
		case meta.Kind == "Ingress" && isAPIGroup(meta.APIVersion, "networking.k8s.io"):
			var ing networkingv1.Ingress
			if err := yaml.UnmarshalStrict(doc, &ing); err != nil {
				return fmt.Errorf("document %d (Ingress): %w", i, err)
			}
			out.ingresses = append(out.ingresses, ing)

		case meta.Kind == "HTTPRoute" && isAPIGroup(meta.APIVersion, "gateway.networking.k8s.io"):
			var route gatewayv1.HTTPRoute
			if err := yaml.UnmarshalStrict(doc, &route); err != nil {
				return fmt.Errorf("document %d (HTTPRoute): %w", i, err)
			}
			out.httpRoutes = append(out.httpRoutes, route)
		}
	}
}

// isAPIGroup reports whether apiVersion ("group/version") is in group.
func isAPIGroup(apiVersion, group string) bool {
	for i := range len(apiVersion) {
		if apiVersion[i] == '/' {
			return apiVersion[:i] == group
		}
	}
	return false // a group-less apiVersion such as "v1" is never one of ours
}
