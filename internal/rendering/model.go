package rendering

type Heading struct {
	Level int    `json:"level"`
	ID    string `json:"id,omitempty"`
	Text  string `json:"text"`
}

type Result struct {
	HTML        string         `json:"html"`
	Text        string         `json:"text"`
	Headings    []Heading      `json:"headings,omitempty"`
	ElementIDs  []string       `json:"elementIds,omitempty"`
	FrontMatter map[string]any `json:"frontMatter,omitempty"`
}
