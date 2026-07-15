/*
---
relationships:

	references: system

---
*/
package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestHelpDocumentsConfigurationAndWorkflows(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		args     []string
		expected []string
	}{
		{
			name: "root",
			args: []string{"--help"},
			expected: []string{
				"lore help <command>", "LORE_PROJECT", "LORE_CONFIG", "LORE_SERVER_URL",
				"flags > environment > credential file > defaults", "lore config",
			},
		},
		{
			name: "config",
			args: []string{"help", "config"},
			expected: []string{
				"Resolution order", "LORE_CONFIG", "$XDG_CONFIG_HOME/lore/config.yml",
				"server:", "ingest-token:", "admin-token:", "Tokens are always redacted",
			},
		},
		{
			name: "create project",
			args: []string{"help", "project", "create"},
			expected: []string{
				"LORE_ADMIN_TOKEN", "admin-token", "idempotent by slug",
				`lore projects create --slug refinery --name "Refinery"`,
			},
		},
		{
			name: "upload notes",
			args: []string{"help", "upload", "notes"},
			expected: []string{
				"LORE_PROJECT", "LORE_INGEST_TOKEN", "--source-instance", "--complete",
				"authoritative projection", "lore upload notes",
			},
		},
		{
			name: "upload conversations",
			args: []string{"help", "upload", "conversations"},
			expected: []string{
				"--provider", "--mapping", "--fallback-project", "allowProjectFallback",
				"LORE_PROJECT", "unassigned sessions are skipped",
			},
		},
		{
			name: "annotations",
			args: []string{"help", "annotations", "export"},
			expected: []string{
				"Annotations are created and managed in the Lore web interface", "LORE_PROJECT",
				"complete snapshot", "nextCursor", "--after", "standard output",
			},
		},
		{
			name: "search",
			args: []string{"help", "search"},
			expected: []string{
				"LORE_PROJECT", "indented JSON", "--source-type", "repeatable or comma-separated",
				"RFC3339", `lore search --tag architecture "knowledge graph"`,
			},
		},
		{
			name: "watch",
			args: []string{"help", "watch"},
			expected: []string{
				"credentials.yml", "watch.yml", "LORE_CONFIG", "complete scan",
				"source-instance", "lore --config credentials.yml watch --config watch.yml",
			},
		},
		{
			name: "briefing contract",
			args: []string{"help", "briefing", "contract"},
			expected: []string{
				"machine-readable", "stylesheet identity", "authoring constraints", "--format json",
			},
		},
		{
			name: "migrate",
			args: []string{"help", "migrate"},
			expected: []string{
				"DATABASE_URL", "do not run automatically", "lore migrate",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var output, errors bytes.Buffer
			if err := New(&output, &errors).Run(context.Background(), test.args); err != nil {
				t.Fatal(err)
			}
			if errors.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", errors.String())
			}
			for _, expected := range test.expected {
				if !strings.Contains(output.String(), expected) {
					t.Errorf("help for %q does not contain %q:\n%s", strings.Join(test.args, " "), expected, output.String())
				}
			}
		})
	}
}

func TestHelpCommandMatchesDirectCommandHelp(t *testing.T) {
	t.Parallel()
	for _, command := range [][]string{
		{"annotations", "export"},
		{"projects", "create"},
		{"upload", "repository"},
		{"briefings", "write-skill"},
		{"search"},
	} {
		t.Run(strings.Join(command, " "), func(t *testing.T) {
			t.Parallel()
			viaHelp := runHelp(t, append([]string{"help"}, command...))
			direct := runHelp(t, append(append([]string{}, command...), "--help"))
			if viaHelp != direct {
				t.Fatalf("lore help output differs from direct --help:\n--- help command ---\n%s\n--- direct ---\n%s", viaHelp, direct)
			}
		})
	}
}

func runHelp(t *testing.T, args []string) string {
	t.Helper()
	var output, errors bytes.Buffer
	if err := New(&output, &errors).Run(context.Background(), args); err != nil {
		t.Fatal(err)
	}
	if errors.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", errors.String())
	}
	return output.String()
}
