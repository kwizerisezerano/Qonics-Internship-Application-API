package config

import "testing"

func TestWeakSecretWarning(t *testing.T) {
	tests := []struct {
		secret string
		weak   bool
	}{
		{"secret", true},
		{"changeme", true},
		{"short-but-not-placeholder", true},
		{"8f3b0c4e9a1d7f2b6c5e8a0d3f1b9c7e2a4d6f8b", false},
	}
	for _, tt := range tests {
		if got := weakSecretWarning(tt.secret) != ""; got != tt.weak {
			t.Errorf("weakSecretWarning(%q) weak = %v, want %v", tt.secret, got, tt.weak)
		}
	}
}
