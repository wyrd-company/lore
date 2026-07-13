package rendering

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"unicode"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func Briefing(source []byte) (Result, error) {
	document, err := xhtml.Parse(bytes.NewReader(source))
	if err != nil {
		return Result{}, fmt.Errorf("parse briefing HTML: %w", err)
	}
	body := findElement(document, "body")
	var nodes []*xhtml.Node
	if body != nil && containsTag(source, "body") {
		for child := body.FirstChild; child != nil; child = child.NextSibling {
			nodes = append(nodes, child)
		}
	} else {
		context := &xhtml.Node{Type: xhtml.ElementNode, DataAtom: atom.Div, Data: "div"}
		nodes, err = xhtml.ParseFragment(bytes.NewReader(source), context)
		if err != nil {
			return Result{}, fmt.Errorf("parse briefing fragment: %w", err)
		}
	}
	var rendered strings.Builder
	for _, node := range nodes {
		if err := xhtml.Render(&rendered, node); err != nil {
			return Result{}, fmt.Errorf("render briefing body: %w", err)
		}
	}
	result := inspectHTML(nodes)
	result.HTML = rendered.String()
	return result, nil
}

func InspectRenderedHTML(rendered string) (Result, error) {
	context := &xhtml.Node{Type: xhtml.ElementNode, DataAtom: atom.Div, Data: "div"}
	nodes, err := xhtml.ParseFragment(strings.NewReader(rendered), context)
	if err != nil {
		return Result{}, fmt.Errorf("parse rendered HTML: %w", err)
	}
	result := inspectHTML(nodes)
	result.HTML = rendered
	return result, nil
}

func inspectHTML(nodes []*xhtml.Node) Result {
	var text strings.Builder
	var headings []Heading
	var ids []string
	for _, node := range nodes {
		walkHTML(node, false, &text, &headings, &ids)
	}
	ids = uniqueSorted(ids)
	return Result{Text: normalizeSpace(text.String()), Headings: headings, ElementIDs: ids}
}

func walkHTML(node *xhtml.Node, hidden bool, text *strings.Builder, headings *[]Heading, ids *[]string) {
	if node.Type == xhtml.ElementNode {
		switch node.Data {
		case "script", "style", "noscript", "head", "template":
			hidden = true
		}
		if id := attribute(node, "id"); id != "" {
			*ids = append(*ids, id)
		}
		if len(node.Data) == 2 && node.Data[0] == 'h' && node.Data[1] >= '1' && node.Data[1] <= '6' {
			*headings = append(*headings, Heading{Level: int(node.Data[1] - '0'), ID: attribute(node, "id"), Text: nodeText(node)})
		}
	}
	if node.Type == xhtml.TextNode && !hidden {
		text.WriteString(node.Data)
		text.WriteByte(' ')
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		walkHTML(child, hidden, text, headings, ids)
	}
}

func findElement(node *xhtml.Node, name string) *xhtml.Node {
	if node.Type == xhtml.ElementNode && node.Data == name {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findElement(child, name); found != nil {
			return found
		}
	}
	return nil
}

func nodeText(node *xhtml.Node) string {
	var value strings.Builder
	var visit func(*xhtml.Node)
	visit = func(current *xhtml.Node) {
		if current.Type == xhtml.TextNode {
			value.WriteString(current.Data)
			value.WriteByte(' ')
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(node)
	return normalizeSpace(value.String())
}

func attribute(node *xhtml.Node, name string) string {
	for _, attr := range node.Attr {
		if attr.Key == name {
			return attr.Val
		}
	}
	return ""
}

func containsTag(source []byte, name string) bool {
	tokenizer := xhtml.NewTokenizer(bytes.NewReader(source))
	for {
		switch tokenizer.Next() {
		case xhtml.ErrorToken:
			return false
		case xhtml.StartTagToken:
			tokenName, _ := tokenizer.TagName()
			if strings.EqualFold(string(tokenName), name) {
				return true
			}
		}
	}
}

func normalizeSpace(value string) string {
	return strings.Join(strings.FieldsFunc(value, unicode.IsSpace), " ")
}

func uniqueSorted(values []string) []string {
	slices.Sort(values)
	return slices.Compact(values)
}
