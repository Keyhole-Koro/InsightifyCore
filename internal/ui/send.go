package ui

import "context"

// SendUpsertNode emits an upsert-node event through the context-bound UI emitter.
// Returns true when an emitter is present and the event is emitted.
func SendUpsertNode(ctx context.Context, node Node) bool {
	emitter := EmitterFrom(ctx)
	if emitter == nil {
		return false
	}
	emitter.EmitUIEvent(Event{
		Type: EventTypeUpsertNode,
		Node: node,
	})
	return true
}
