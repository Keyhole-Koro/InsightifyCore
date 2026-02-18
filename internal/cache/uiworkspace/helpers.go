package uiworkspace

import "strings"

func normalizeProjectID(v string) string   { return strings.TrimSpace(v) }
func normalizeWorkspaceID(v string) string { return strings.TrimSpace(v) }
func normalizeTabID(v string) string       { return strings.TrimSpace(v) }
func normalizeRunID(v string) string       { return strings.TrimSpace(v) }

func normalizeTitle(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "Tab"
	}
	return v
}
