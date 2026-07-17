package ai

import "testing"

func TestConnectionRequiresAuthenticationRejectsSpoofedLoopbackURL(t *testing.T) {
	for _, test := range []struct {
		url      string
		requires bool
	}{
		{url: "http://127.0.0.1:11434", requires: false},
		{url: "http://[::1]:11434", requires: false},
		{url: "http://127.0.0.1.evil:11434", requires: true},
	} {
		t.Run(test.url, func(t *testing.T) {
			if got := ConnectionRequiresAuthentication("ollama", "none", test.url); got != test.requires {
				t.Fatalf("ConnectionRequiresAuthentication(%q) = %t, want %t", test.url, got, test.requires)
			}
		})
	}
}
