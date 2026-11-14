package v1alpha1

import "testing"

func TestObjectRefString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ref  ObjectRef
		want string
	}{
		{
			name: "namespaced with group",
			ref:  ObjectRef{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute", Namespace: "shop", Name: "web"},
			want: "HTTPRoute.gateway.networking.k8s.io shop/web",
		},
		{
			name: "core group is omitted",
			ref:  ObjectRef{Kind: "Ingress", Namespace: "shop", Name: "web"},
			want: "Ingress shop/web",
		},
		{
			name: "cluster scoped",
			ref:  ObjectRef{Kind: "GatewayClass", Name: "envoy"},
			want: "GatewayClass envoy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.ref.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseSeverity(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"critical", "major", "minor", "info"} {
		got, err := ParseSeverity(s)
		if err != nil {
			t.Errorf("ParseSeverity(%q): %v", s, err)
		}
		if got.String() != s {
			t.Errorf("ParseSeverity(%q) = %q", s, got)
		}
	}
	for _, s := range []string{"", "CRITICAL", "fatal", "warning"} {
		if _, err := ParseSeverity(s); err == nil {
			t.Errorf("ParseSeverity(%q) = nil error, want one", s)
		}
	}
}

func TestSeverityAtLeast(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sev       Severity
		threshold Severity
		want      bool
	}{
		{name: "equal", sev: SeverityMajor, threshold: SeverityMajor, want: true},
		{name: "above", sev: SeverityCritical, threshold: SeverityMajor, want: true},
		{name: "below", sev: SeverityMinor, threshold: SeverityMajor, want: false},
		{name: "info never meets major", sev: SeverityInfo, threshold: SeverityMajor, want: false},
		{name: "info meets info", sev: SeverityInfo, threshold: SeverityInfo, want: true},
		// An unset or unknown severity must never fail a run, whatever the
		// threshold: --fail-on only reacts to severities we assigned on purpose.
		{name: "empty against info", sev: "", threshold: SeverityInfo, want: false},
		{name: "empty against empty", sev: "", threshold: "", want: false},
		{name: "unknown against empty", sev: "fatal", threshold: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.sev.AtLeast(tt.threshold); got != tt.want {
				t.Errorf("%q.AtLeast(%q) = %v, want %v", tt.sev, tt.threshold, got, tt.want)
			}
		})
	}
}
