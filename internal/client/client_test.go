package client

import "testing"

func TestNormalizeServerURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		input       string
		expected    string
		assumedHTTP bool
	}{
		{name: "docker service", input: "lore:8080", expected: "http://lore:8080", assumedHTTP: true},
		{name: "hostname", input: "localhost", expected: "http://localhost", assumedHTTP: true},
		{name: "explicit HTTP", input: "http://lore:8080", expected: "http://lore:8080"},
		{name: "explicit HTTPS", input: "https://lore.example.net/", expected: "https://lore.example.net"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			normalized, assumedHTTP, err := NormalizeServerURL(tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if normalized != tt.expected || assumedHTTP != tt.assumedHTTP {
				t.Fatalf("NormalizeServerURL(%q) = %q, %t; want %q, %t", tt.input, normalized, assumedHTTP, tt.expected, tt.assumedHTTP)
			}
		})
	}
}

func TestNormalizeServerURLRejectsInvalidAndUnsupportedURLs(t *testing.T) {
	t.Parallel()
	for _, input := range []string{"", "http://", "ftp://lore:8080"} {
		if _, _, err := NormalizeServerURL(input); err == nil {
			t.Errorf("NormalizeServerURL(%q) succeeded", input)
		}
	}
}
