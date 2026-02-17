package utils

import (
	"sort"
	"strings"
)

// PathsToTree converts a list of file paths into a visual tree string.
// Example:
// src
// ├── main.go
// └── utils
//     └── helper.go
func PathsToTree(paths []string) string {
	if len(paths) == 0 {
		return ""
	}

	// 1. Build a nested map structure
	root := make(map[string]any)
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		parts := strings.Split(p, "/")
		current := root
		for _, part := range parts {
			if part == "" || part == "." {
				continue
			}
			if _, ok := current[part]; !ok {
				current[part] = make(map[string]any)
			}
			current = current[part].(map[string]any)
		}
	}

	// 2. Render the tree
	var sb strings.Builder
	renderTree(&sb, root, "")
	return strings.TrimSpace(sb.String())
}

func renderTree(sb *strings.Builder, node map[string]any, prefix string) {
	keys := make([]string, 0, len(node))
	for k := range node {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for i, k := range keys {
		isLast := i == len(keys)-1
		sb.WriteString(prefix)
		if isLast {
			sb.WriteString("└── ")
		} else {
			sb.WriteString("├── ")
		}
		sb.WriteString(k)
		sb.WriteString("\n")

		children := node[k].(map[string]any)
		if len(children) > 0 {
			newPrefix := prefix
			if isLast {
				newPrefix += "    "
			} else {
				newPrefix += "│   "
			}
			renderTree(sb, children, newPrefix)
		}
	}
}
