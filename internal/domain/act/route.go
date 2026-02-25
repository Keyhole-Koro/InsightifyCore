package act

import "strings"

// RouteResult holds the outcome of rule-based input classification.
type RouteResult struct {
	// WorkerKey is the selected worker to execute (e.g. "bootstrap").
	WorkerKey string
	// Mode is the act operational mode: "suggest", "search", or "run_worker".
	Mode string
	// Confidence is a score in [0.0, 1.0] reflecting classification certainty.
	Confidence float64
}

// suggestKeywords triggers suggest mode.
var suggestKeywords = []string{
	"suggest", "propose", "idea", "recommend", "advice",
	"提案", "おすすめ", "案",
}

// searchKeywords triggers search mode.
var searchKeywords = []string{
	"search", "find", "look", "lookup", "discover", "query",
	"検索", "探す", "探し", "調べ",
}

// RouteInput classifies user input text and returns the recommended worker,
// act mode, and confidence score using deterministic keyword matching.
func RouteInput(goal string) RouteResult {
	g := strings.ToLower(strings.TrimSpace(goal))
	if g == "" {
		return RouteResult{WorkerKey: "bootstrap", Mode: "run_worker", Confidence: 0.1}
	}

	suggestHit := matchesAny(g, suggestKeywords)
	searchHit := matchesAny(g, searchKeywords)

	switch {
	case suggestHit && searchHit:
		// Ambiguous: both categories match → low confidence, default to suggest.
		return RouteResult{WorkerKey: "bootstrap", Mode: "suggest", Confidence: 0.4}
	case suggestHit:
		return RouteResult{WorkerKey: "bootstrap", Mode: "suggest", Confidence: 0.8}
	case searchHit:
		return RouteResult{WorkerKey: "bootstrap", Mode: "search", Confidence: 0.8}
	default:
		// No specific keyword → default to run_worker.
		return RouteResult{WorkerKey: "bootstrap", Mode: "run_worker", Confidence: 0.6}
	}
}

// FallbackConfidenceThreshold is the threshold below which routing falls back
// to the autonomous_executor.
const FallbackConfidenceThreshold = 0.5

// ShouldFallback returns true when classification confidence is below the
// threshold and the act should use the autonomous_executor as fallback.
func ShouldFallback(confidence float64) bool {
	return confidence < FallbackConfidenceThreshold
}

// IsWorkerAllowed returns true if key is in the allowed list, or if the
// allowed list is empty (all workers permitted).
func IsWorkerAllowed(key string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	k := strings.TrimSpace(strings.ToLower(key))
	for _, a := range allowed {
		if strings.TrimSpace(strings.ToLower(a)) == k {
			return true
		}
	}
	return false
}

func matchesAny(text string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}
