package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestApplicationShellRestrictsTrustedRenderedContentCapabilities(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	New(nil, "ingest", "admin").ServeHTTP(response, request)
	policy := response.Header().Get("Content-Security-Policy")
	for _, directive := range []string{"script-src 'self'", "object-src 'none'", "frame-ancestors 'none'", "frame-src 'none'", "form-action 'none'", "connect-src 'self'"} {
		if !strings.Contains(policy, directive) {
			t.Errorf("Content-Security-Policy %q does not contain %q", policy, directive)
		}
	}
	if strings.Contains(policy, "script-src 'self' 'unsafe-inline'") {
		t.Fatalf("inline scripts are allowed by policy: %s", policy)
	}
	if got := response.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", got)
	}
}
