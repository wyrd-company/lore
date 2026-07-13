package rendering

import (
	"bytes"
	"fmt"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"gopkg.in/yaml.v3"
)

var markdownEngine = goldmark.New(
	goldmark.WithExtensions(
		extension.GFM,
		extension.Footnote,
		highlighting.NewHighlighting(highlighting.WithFormatOptions(chromahtml.WithClasses(true))),
	),
	goldmark.WithParserOptions(parser.WithAutoHeadingID()),
)

func Markdown(source []byte) (Result, error) {
	body, frontMatter, err := splitFrontMatter(source)
	if err != nil {
		return Result{}, err
	}
	var rendered bytes.Buffer
	if err := markdownEngine.Convert(body, &rendered); err != nil {
		return Result{}, fmt.Errorf("render Markdown: %w", err)
	}
	result, err := InspectRenderedHTML(rendered.String())
	if err != nil {
		return Result{}, err
	}
	result.FrontMatter = frontMatter
	return result, nil
}

func splitFrontMatter(source []byte) ([]byte, map[string]any, error) {
	if !bytes.HasPrefix(source, []byte("---\n")) && !bytes.HasPrefix(source, []byte("---\r\n")) {
		return source, nil, nil
	}
	normalized := bytes.ReplaceAll(source, []byte("\r\n"), []byte("\n"))
	remainder := normalized[4:]
	end := bytes.Index(remainder, []byte("\n---\n"))
	if end < 0 {
		return nil, nil, fmt.Errorf("Markdown front matter is not terminated")
	}
	frontMatter := make(map[string]any)
	if err := yaml.Unmarshal(remainder[:end], &frontMatter); err != nil {
		return nil, nil, fmt.Errorf("parse Markdown front matter: %w", err)
	}
	return remainder[end+5:], frontMatter, nil
}
