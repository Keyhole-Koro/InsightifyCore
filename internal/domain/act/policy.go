package act

import "strings"

// IsNodeCreateActorAllowed checks whether actor is allowed to create UI nodes.
func IsNodeCreateActorAllowed(actor string) bool {
	switch strings.TrimSpace(strings.ToLower(actor)) {
	case "act", "worker", "system":
		return true
	default:
		return false
	}
}
