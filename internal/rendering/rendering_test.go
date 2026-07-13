package rendering

import (
	"strings"
	"testing"
)

func TestMarkdownSupportsContractFeatures(t *testing.T) {
	source := []byte(`---
title: Example
tags: [one, two]
---
# Heading

## Repeat
## Repeat

- [x] complete

| A | B |
|---|---|
| 1 | 2 |

Footnote[^1].

[^1]: Detail.

` + "```go\nfmt.Println(\"hello\")\n```\n")
	result, err := Markdown(source)
	if err != nil {
		t.Fatalf("render Markdown: %v", err)
	}
	for _, expected := range []string{`id="heading"`, `id="repeat"`, `id="repeat-1"`, `<table>`, `type="checkbox"`, `class="chroma"`, `class="footnotes"`} {
		if !strings.Contains(result.HTML, expected) {
			t.Errorf("rendered Markdown missing %q:\n%s", expected, result.HTML)
		}
	}
	if result.FrontMatter["title"] != "Example" {
		t.Fatalf("front matter title = %#v", result.FrontMatter["title"])
	}
	if strings.Contains(result.Text, "title Example") || !strings.Contains(result.Text, "Heading") {
		t.Fatalf("unexpected normalized text %q", result.Text)
	}
}

func TestBriefingExtractsBodyAndSupportsFragments(t *testing.T) {
	result, err := Briefing([]byte(`<!doctype html><html><head><title>Ignore me</title><style>.bad{}</style></head><body><h1 id="overview">Overview</h1><p>Visible text.</p><script>ignore()</script></body></html>`))
	if err != nil {
		t.Fatalf("render briefing: %v", err)
	}
	if strings.Contains(result.HTML, "Ignore me") || strings.Contains(result.Text, "ignore") {
		t.Fatalf("head or scripts leaked: %#v", result)
	}
	if result.Text != "Overview Visible text." || len(result.ElementIDs) != 1 || result.ElementIDs[0] != "overview" {
		t.Fatalf("unexpected briefing extraction: %#v", result)
	}

	fragment, err := Briefing([]byte(`<h1>Fragment</h1><p>Works.</p>`))
	if err != nil || fragment.Text != "Fragment Works." {
		t.Fatalf("briefing fragment = %#v, %v", fragment, err)
	}
}

func TestYAMLProducesStructuralHTML(t *testing.T) {
	result, err := YAML([]byte("project:\n  name: Lore\n  tags: [one, two]\n  sources:\n    - path: /one\n      type: note\n    - path: /two\n      type: task\n"))
	if err != nil {
		t.Fatalf("render YAML: %v", err)
	}
	for _, expected := range []string{`class="lore-struct"`, `<h1 id="project">project</h1>`, `<h2 id="project-name">name</h2>`, `<ul><li>one</li><li>two</li></ul>`, `data-yaml-index="1"`} {
		if !strings.Contains(result.HTML, expected) {
			t.Errorf("structural HTML missing %q:\n%s", expected, result.HTML)
		}
	}
}

func TestTaskAndConversationUseLoreContractClasses(t *testing.T) {
	task, err := Task(TaskModel{Title: "Build", Status: "todo", Priority: "high", Body: "**Description**", DependsOn: []string{"1"}})
	if err != nil || !strings.Contains(task.HTML, `class="lore-task-meta"`) || !strings.Contains(task.HTML, `class="lore-deps"`) {
		t.Fatalf("task result = %#v, %v", task, err)
	}

	conversation, err := Conversation([]Message{
		{ID: "message-1", Role: "user", Markdown: "Hello"},
		{ID: "thinking-1", Role: "assistant", Markdown: "Consider this", Thinking: true},
	})
	if err != nil {
		t.Fatalf("render conversation: %v", err)
	}
	if !strings.Contains(conversation.HTML, `class="lore-msg"`) || !strings.Contains(conversation.HTML, `<details class="lore-thinking"`) || !strings.Contains(conversation.Text, "Thinking: Consider this") {
		t.Fatalf("unexpected conversation: %#v", conversation)
	}
}
