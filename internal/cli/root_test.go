package cli

import "testing"

func TestNewLogger(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		level, format string
		wantErr       bool
	}{
		{name: "defaults", level: "info", format: "text"},
		{name: "json", level: "debug", format: "json"},
		{name: "format is case insensitive", level: "warn", format: "JSON"},
		{name: "bad level", level: "loud", format: "text", wantErr: true},
		{name: "bad format", level: "info", format: "yaml", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := newLogger(tt.level, tt.format)
			if (err != nil) != tt.wantErr {
				t.Fatalf("newLogger(%q, %q) error = %v, wantErr %v", tt.level, tt.format, err, tt.wantErr)
			}
			if err == nil && got == nil {
				t.Fatal("newLogger returned nil logger without an error")
			}
		})
	}
}
