package cli

import (
	"bytes"
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wyrd-company/lore/internal/httpapi"
)

func TestClientConfigResolutionOrder(t *testing.T) {
	clearClientEnvironment(t)
	path := filepath.Join(t.TempDir(), "credentials.yml")
	writeClientConfig(t, path, "server: https://config.example\ningest-token: config-ingest\nadmin-token: config-admin\n")
	t.Setenv("LORE_CONFIG", path)
	t.Setenv("LORE_SERVER_URL", "https://environment.example")
	t.Setenv("LORE_INGEST_TOKEN", "environment-ingest")
	t.Setenv("LORE_ADMIN_TOKEN", "environment-admin")

	resolved, err := resolveClientConfig(configSelection{})
	if err != nil {
		t.Fatal(err)
	}
	assertResolvedValue(t, resolved.ServerURL, "https://environment.example", "environment LORE_SERVER_URL")
	assertResolvedValue(t, resolved.IngestToken, "environment-ingest", "environment LORE_INGEST_TOKEN")
	assertResolvedValue(t, resolved.AdminToken, "environment-admin", "environment LORE_ADMIN_TOKEN")
	if len(resolved.Lookups) != 1 || !resolved.Lookups[0].Loaded || resolved.Lookups[0].Path != path {
		t.Fatalf("config lookups = %#v", resolved.Lookups)
	}
}

func TestClientConfigDefaultSearchAndPartialFiles(t *testing.T) {
	clearClientEnvironment(t)
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	path := filepath.Join(xdg, "lore", "config.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	writeClientConfig(t, path, "ingest-token: xdg-ingest\n")

	paths := configPaths(configSelection{})
	if len(paths) != 2 || paths[0] != path || paths[1] != "/etc/lore/config.yml" {
		t.Fatalf("default config search = %#v", paths)
	}
	resolved, err := resolveClientConfig(configSelection{Path: path, Explicit: true})
	if err != nil {
		t.Fatal(err)
	}
	assertResolvedValue(t, resolved.ServerURL, defaultServerURL, "default")
	assertResolvedValue(t, resolved.IngestToken, "xdg-ingest", "config "+path)
	if resolved.AdminToken.Value != "" {
		t.Fatalf("admin token = %q, want empty", resolved.AdminToken.Value)
	}
	if len(resolved.Lookups) != 1 || resolved.Lookups[0].Path != path || !resolved.Lookups[0].Loaded {
		t.Fatalf("explicit partial config lookup = %#v", resolved.Lookups)
	}
}

func TestClientCredentialsThroughRealServerAndPostgres(t *testing.T) {
	clearClientEnvironment(t)
	t.Setenv("LORE_PROJECT", "config-only")
	pool := annotationExportPool(t)
	ctx := context.Background()
	fixtures := filepath.Join("..", "adapters", "testdata", "notes")
	var output bytes.Buffer
	runner := New(&output, &output)

	configServer := httptest.NewServer(httpapi.New(pool, "config-ingest", "config-admin"))
	t.Cleanup(configServer.Close)
	configPath := filepath.Join(t.TempDir(), "config.yml")
	writeClientConfig(t, configPath, fmt.Sprintf("server: %s\ningest-token: config-ingest\nadmin-token: config-admin\n", configServer.URL))
	if err := os.Chmod(configPath, 0o444); err != nil {
		t.Fatal(err)
	}
	runClientCommand(t, runner, ctx, "projects", "create", "--config", configPath, "--slug", "config-only", "--name", "Config only")
	runClientCommand(t, runner, ctx, "upload", "notes", "--config", configPath, "--source-instance", "config-notes", fixtures)

	environmentServer := httptest.NewServer(httpapi.New(pool, "environment-ingest", "environment-admin"))
	t.Cleanup(environmentServer.Close)
	t.Setenv("LORE_SERVER_URL", environmentServer.URL)
	t.Setenv("LORE_INGEST_TOKEN", "environment-ingest")
	t.Setenv("LORE_ADMIN_TOKEN", "environment-admin")
	t.Setenv("LORE_PROJECT", "environment")
	wrongConfig := filepath.Join(t.TempDir(), "wrong.yml")
	writeClientConfig(t, wrongConfig, "server: http://127.0.0.1:1\ningest-token: wrong-ingest\nadmin-token: wrong-admin\n")
	runClientCommand(t, runner, ctx, "projects", "create", "--config", wrongConfig, "--slug", "environment", "--name", "Environment")
	runClientCommand(t, runner, ctx, "upload", "notes", "--config", wrongConfig, "--source-instance", "environment-notes", fixtures)

	flagServer := httptest.NewServer(httpapi.New(pool, "flag-ingest", "flag-admin"))
	t.Cleanup(flagServer.Close)
	t.Setenv("LORE_SERVER_URL", "http://127.0.0.1:1")
	t.Setenv("LORE_INGEST_TOKEN", "wrong-environment-ingest")
	t.Setenv("LORE_ADMIN_TOKEN", "wrong-environment-admin")
	missingConfig := filepath.Join(t.TempDir(), "missing.yml")
	runClientCommand(t, runner, ctx, "projects", "create", "--config", missingConfig, "--server", flagServer.URL, "--token", "flag-admin", "--slug", "flags", "--name", "Flags")
	runClientCommand(t, runner, ctx, "upload", "notes", "--config", missingConfig, "--server", flagServer.URL, "--token", "flag-ingest", "--project", "flags", "--source-instance", "flag-notes", fixtures)

	var projects, documents int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM projects), (SELECT count(*) FROM documents WHERE deleted_at IS NULL)`).Scan(&projects, &documents); err != nil {
		t.Fatal(err)
	}
	if projects != 3 || documents != 3 {
		t.Fatalf("config integration persistence: projects=%d documents=%d", projects, documents)
	}

	output.Reset()
	showIngest := "show-ingest-secret"
	showAdmin := "show-admin-secret"
	if err := runner.Run(ctx, []string{"config", "--config", configPath, "--server", flagServer.URL, "--ingest-token", showIngest, "--admin-token", showAdmin}); err != nil {
		t.Fatal(err)
	}
	shown := output.String()
	for _, secret := range []string{"config-ingest", "config-admin", showIngest, showAdmin} {
		if strings.Contains(shown, secret) {
			t.Fatalf("config output exposed token %q: %s", secret, shown)
		}
	}
	for _, expected := range []string{configPath + " (loaded)", "ingest-token: <redacted> (flag --ingest-token)", "admin-token: <redacted> (flag --admin-token)", "server: " + flagServer.URL + " (flag --server)"} {
		if !strings.Contains(shown, expected) {
			t.Fatalf("config output missing %q: %s", expected, shown)
		}
	}
}

func TestMissingCredentialNamesEveryLookup(t *testing.T) {
	clearClientEnvironment(t)
	missing := filepath.Join(t.TempDir(), "missing.yml")
	resolved, err := resolveClientConfig(configSelection{Path: missing, Explicit: true})
	if err != nil {
		t.Fatal(err)
	}
	err = resolved.missingCredential("Lore ingest token", "--token", "LORE_INGEST_TOKEN", "ingest-token")
	for _, expected := range []string{"Lore ingest token is required", "--token", "LORE_INGEST_TOKEN", missing + " (missing)"} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("missing credential error lacks %q: %v", expected, err)
		}
	}
}

func TestClientConfigRejectsProjectSetting(t *testing.T) {
	_, err := decodeClientConfig([]byte("project: lore\n"))
	if err == nil || !strings.Contains(err.Error(), "field project not found") {
		t.Fatalf("project setting error = %v", err)
	}
}

func TestClientConfigFlagRejectsEmptyPath(t *testing.T) {
	if _, _, err := extractGlobalConfig([]string{"--config=", "config"}); err == nil {
		t.Fatal("empty global --config path was accepted")
	}
	if _, err := selectionFromArgs([]string{"--config", ""}, configSelection{}); err == nil {
		t.Fatal("empty command --config path was accepted")
	}
}

func clearClientEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{"LORE_CONFIG", "LORE_SERVER_URL", "PUBLIC_BASE_URL", "LORE_INGEST_TOKEN", "LORE_ADMIN_TOKEN", "XDG_CONFIG_HOME"} {
		t.Setenv(name, "")
	}
	t.Setenv("HOME", t.TempDir())
}

func writeClientConfig(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertResolvedValue(t *testing.T, value resolvedValue, expectedValue, expectedSource string) {
	t.Helper()
	if value.Value != expectedValue || value.Source != expectedSource {
		t.Fatalf("resolved value = %#v, want value=%q source=%q", value, expectedValue, expectedSource)
	}
}

func runClientCommand(t *testing.T, runner *Runner, ctx context.Context, arguments ...string) {
	t.Helper()
	if err := runner.Run(ctx, arguments); err != nil {
		t.Fatalf("lore %s: %v", strings.Join(arguments, " "), err)
	}
}
