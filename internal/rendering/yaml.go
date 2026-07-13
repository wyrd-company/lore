package rendering

import (
	"fmt"
	"html"
	"strconv"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

func YAML(source []byte) (Result, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(source, &document); err != nil {
		return Result{}, fmt.Errorf("parse YAML: %w", err)
	}
	if len(document.Content) == 0 {
		return Result{HTML: `<div class="lore-struct"></div>`}, nil
	}
	renderer := yamlRenderer{slugCounts: make(map[string]int)}
	renderer.output.WriteString(`<div class="lore-struct">`)
	renderer.renderNode(document.Content[0], nil, 1)
	renderer.output.WriteString(`</div>`)
	result, err := InspectRenderedHTML(renderer.output.String())
	if err != nil {
		return Result{}, err
	}
	return result, nil
}

type yamlRenderer struct {
	output     strings.Builder
	slugCounts map[string]int
}

func (r *yamlRenderer) renderNode(node *yaml.Node, path []string, depth int) {
	switch node.Kind {
	case yaml.MappingNode:
		for index := 0; index+1 < len(node.Content); index += 2 {
			key := node.Content[index].Value
			value := node.Content[index+1]
			currentPath := appendPath(path, key)
			headingLevel := min(depth, 6)
			id := r.uniqueSlug(strings.Join(currentPath, "-"))
			fmt.Fprintf(&r.output, `<section data-yaml-path="%s"><h%d id="%s">%s</h%d>`,
				html.EscapeString(strings.Join(currentPath, ".")), headingLevel, html.EscapeString(id), html.EscapeString(key), headingLevel)
			r.renderNode(value, currentPath, depth+1)
			r.output.WriteString(`</section>`)
		}
	case yaml.SequenceNode:
		if allMappings(node.Content) {
			for index, item := range node.Content {
				fmt.Fprintf(&r.output, `<section class="lore-struct__item" data-yaml-index="%d">`, index)
				r.renderNode(item, appendPath(path, strconv.Itoa(index)), depth)
				r.output.WriteString(`</section>`)
			}
			return
		}
		r.output.WriteString(`<ul>`)
		for index, item := range node.Content {
			r.output.WriteString(`<li>`)
			if item.Kind == yaml.ScalarNode {
				r.output.WriteString(html.EscapeString(item.Value))
			} else {
				r.renderNode(item, appendPath(path, strconv.Itoa(index)), depth)
			}
			r.output.WriteString(`</li>`)
		}
		r.output.WriteString(`</ul>`)
	case yaml.ScalarNode:
		fmt.Fprintf(&r.output, `<p>%s</p>`, html.EscapeString(node.Value))
	case yaml.AliasNode:
		if node.Alias != nil {
			r.renderNode(node.Alias, path, depth)
		}
	}
}

func (r *yamlRenderer) uniqueSlug(value string) string {
	var slug strings.Builder
	lastDash := false
	for _, character := range strings.ToLower(value) {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			slug.WriteRune(character)
			lastDash = false
		} else if !lastDash && slug.Len() > 0 {
			slug.WriteByte('-')
			lastDash = true
		}
	}
	base := strings.Trim(slug.String(), "-")
	if base == "" {
		base = "section"
	}
	count := r.slugCounts[base]
	r.slugCounts[base] = count + 1
	if count == 0 {
		return base
	}
	return base + "-" + strconv.Itoa(count+1)
}

func allMappings(nodes []*yaml.Node) bool {
	return len(nodes) > 0 && !containsNonMapping(nodes)
}

func containsNonMapping(nodes []*yaml.Node) bool {
	for _, node := range nodes {
		if node.Kind != yaml.MappingNode {
			return true
		}
	}
	return false
}

func appendPath(path []string, value string) []string {
	result := make([]string, len(path), len(path)+1)
	copy(result, path)
	return append(result, value)
}
