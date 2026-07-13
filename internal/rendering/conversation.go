package rendering

import (
	"fmt"
	"html"
	"strings"
)

type Message struct {
	ID       string `json:"id"`
	Role     string `json:"role"`
	Markdown string `json:"markdown"`
	Thinking bool   `json:"thinking,omitempty"`
}

func Conversation(messages []Message) (Result, error) {
	var rendered strings.Builder
	var normalized strings.Builder
	rendered.WriteString(`<div class="lore-convo">`)
	for _, message := range messages {
		body, err := Markdown([]byte(message.Markdown))
		if err != nil {
			return Result{}, fmt.Errorf("render conversation message %q: %w", message.ID, err)
		}
		if message.Thinking {
			fmt.Fprintf(&rendered, `<details class="lore-thinking" id="%s"><summary>Assistant thinking</summary><div class="lore-prose">%s</div></details>`, html.EscapeString(message.ID), body.HTML)
			normalized.WriteString("Thinking: ")
		} else {
			fmt.Fprintf(&rendered, `<article class="lore-msg" data-role="%s" id="%s"><header>%s</header><div class="lore-msg__body lore-prose">%s</div></article>`,
				html.EscapeString(message.Role), html.EscapeString(message.ID), html.EscapeString(roleLabel(message.Role)), body.HTML)
			normalized.WriteString(message.Role)
			normalized.WriteString(": ")
		}
		normalized.WriteString(body.Text)
		normalized.WriteByte('\n')
	}
	rendered.WriteString(`</div>`)
	result, err := InspectRenderedHTML(rendered.String())
	if err != nil {
		return Result{}, err
	}
	result.Text = strings.TrimSpace(normalized.String())
	return result, nil
}

func roleLabel(role string) string {
	if role == "" {
		return "Message"
	}
	return strings.ToUpper(role[:1]) + role[1:]
}
