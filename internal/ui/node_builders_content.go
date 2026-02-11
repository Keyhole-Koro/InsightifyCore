package ui

import "strings"

func BuildMarkdownNode(id, title, markdown string) (Node, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Node{}, false
	}
	return Node{
		ID:   id,
		Type: NodeTypeMarkdown,
		Meta: Meta{Title: strings.TrimSpace(title)},
		Markdown: &MarkdownState{
			Markdown: markdown,
		},
	}, true
}

func BuildImageNode(id, title, src, alt string) (Node, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Node{}, false
	}
	return Node{
		ID:   id,
		Type: NodeTypeImage,
		Meta: Meta{Title: strings.TrimSpace(title)},
		Image: &ImageState{
			Src: strings.TrimSpace(src),
			Alt: strings.TrimSpace(alt),
		},
	}, true
}

func BuildTableNode(id, title string, columns []string, rows [][]string) (Node, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Node{}, false
	}
	return Node{
		ID:   id,
		Type: NodeTypeTable,
		Meta: Meta{Title: strings.TrimSpace(title)},
		Table: &TableState{
			Columns: append([]string(nil), columns...),
			Rows:    append([][]string(nil), rows...),
		},
	}, true
}
