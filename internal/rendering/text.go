package rendering

import (
	"fmt"
	"html"
)

func Text(source []byte, language string) Result {
	class := ""
	if language != "" {
		class = fmt.Sprintf(` class="language-%s"`, html.EscapeString(language))
	}
	rendered := fmt.Sprintf(`<pre><code%s>%s</code></pre>`, class, html.EscapeString(string(source)))
	return Result{HTML: rendered, Text: normalizeSpace(string(source))}
}
