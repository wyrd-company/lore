package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestHelpCommandsExitZero(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args []string
	}{
		{name: "long flag", args: []string{"--help"}},
		{name: "short flag", args: []string{"-h"}},
		{name: "help command", args: []string{"help"}},
		{name: "navigated help", args: []string{"help", "annotations", "export"}},
		{name: "project group", args: []string{"project", "--help"}},
		{name: "project command", args: []string{"projects", "create", "--help"}},
		{name: "upload group", args: []string{"upload", "--help"}},
		{name: "task upload", args: []string{"upload", "tasks", "--help"}},
		{name: "note upload", args: []string{"upload", "notes", "--help"}},
		{name: "briefing upload", args: []string{"upload", "briefing", "--help"}},
		{name: "repository upload", args: []string{"upload", "repository", "--help"}},
		{name: "conversation upload", args: []string{"upload", "conversations", "--help"}},
		{name: "annotation group", args: []string{"annotations", "--help"}},
		{name: "annotation command", args: []string{"annotation", "export", "--help"}},
		{name: "briefing group", args: []string{"briefings", "--help"}},
		{name: "briefing show css", args: []string{"briefing", "show-css", "--help"}},
		{name: "briefing show skill", args: []string{"briefing", "show-skill", "--help"}},
		{name: "briefing write css", args: []string{"briefing", "write-css", "--help"}},
		{name: "briefing write skill", args: []string{"briefing", "write-skill", "--help"}},
		{name: "briefing contract", args: []string{"briefing", "contract", "--help"}},
		{name: "search command", args: []string{"search", "--help"}},
		{name: "config command", args: []string{"config", "--help"}},
		{name: "watch command", args: []string{"watch", "--help"}},
		{name: "migrate command", args: []string{"migrate", "--help"}},
		{name: "version command", args: []string{"version", "--help"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			if code := run(tt.args, &stdout, &stderr); code != 0 {
				t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
			}
			if !strings.Contains(stdout.String(), "Usage:") {
				t.Fatalf("stdout does not contain usage: %q", stdout.String())
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}
		})
	}
}

func TestUnknownHelpTopicExitsOneWithRootUsageOnStderr(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	if code := run([]string{"help", "unknown"}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	for _, expected := range []string{`error: unknown help topic "unknown"`, "lore help <command>"} {
		if !strings.Contains(stderr.String(), expected) {
			t.Fatalf("stderr does not contain %q: %q", expected, stderr.String())
		}
	}
}

func TestUnknownCommandExitsOneWithErrorAndUsageOnStderr(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	if code := run([]string{"unknown"}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	for _, expected := range []string{`error: unknown command "unknown"`, "Usage:"} {
		if !strings.Contains(stderr.String(), expected) {
			t.Fatalf("stderr does not contain %q: %q", expected, stderr.String())
		}
	}
}
