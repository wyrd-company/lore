package rendering

import (
	"fmt"
	"html"
	"strings"
)

type TaskModel struct {
	ID        string
	Title     string
	Status    string
	Priority  string
	Assignee  string
	Body      string
	Tags      []string
	DependsOn []string
}

func Task(task TaskModel) (Result, error) {
	body, err := Markdown([]byte(task.Body))
	if err != nil {
		return Result{}, err
	}
	var rendered strings.Builder
	rendered.WriteString(`<article class="lore-task">`)
	fmt.Fprintf(&rendered, `<dl class="lore-task-meta"><dt>Status</dt><dd>%s</dd><dt>Priority</dt><dd>%s</dd>`, html.EscapeString(task.Status), html.EscapeString(task.Priority))
	if task.Assignee != "" {
		fmt.Fprintf(&rendered, `<dt>Assignee</dt><dd>%s</dd>`, html.EscapeString(task.Assignee))
	}
	rendered.WriteString(`</dl>`)
	if len(task.DependsOn) > 0 {
		rendered.WriteString(`<nav class="lore-deps" aria-label="Dependencies"><h2>Dependencies</h2><ul>`)
		for _, dependency := range task.DependsOn {
			fmt.Fprintf(&rendered, `<li><span class="lore-dep-link" data-task-id="%s">Task %s</span></li>`, html.EscapeString(dependency), html.EscapeString(dependency))
		}
		rendered.WriteString(`</ul></nav>`)
	}
	fmt.Fprintf(&rendered, `<div class="lore-prose">%s</div></article>`, body.HTML)
	result, err := InspectRenderedHTML(rendered.String())
	if err != nil {
		return Result{}, err
	}
	result.Text = normalizeSpace(strings.Join([]string{task.Title, task.Status, task.Priority, task.Assignee, strings.Join(task.Tags, " "), body.Text}, " "))
	return result, nil
}
