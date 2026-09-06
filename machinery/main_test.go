package main

import "testing"

func TestResolveServerPort(t *testing.T) {
	tests := []struct {
		name      string
		flagValue string
		envValue  string
		want      string
		wantError bool
	}{
		{name: "flag default", flagValue: "80", want: "80"},
		{name: "environment overrides flag", flagValue: "80", envValue: "8082", want: "8082"},
		{name: "trims environment", flagValue: "80", envValue: " 9090 ", want: "9090"},
		{name: "empty values use default", want: "80"},
		{name: "invalid text", flagValue: "80", envValue: "http", wantError: true},
		{name: "zero", flagValue: "80", envValue: "0", wantError: true},
		{name: "above maximum", flagValue: "80", envValue: "65536", wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := resolveServerPort(test.flagValue, test.envValue)
			if (err != nil) != test.wantError {
				t.Fatalf("resolveServerPort(%q, %q) error = %v, wantError %t", test.flagValue, test.envValue, err, test.wantError)
			}
			if got != test.want {
				t.Fatalf("resolveServerPort(%q, %q) = %q, want %q", test.flagValue, test.envValue, got, test.want)
			}
		})
	}
}
