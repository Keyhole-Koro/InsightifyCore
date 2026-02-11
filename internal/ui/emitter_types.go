package ui

type EventType string

const (
	EventTypeUpsertNode EventType = "upsert_node"
	EventTypeRemoveNode EventType = "remove_node"
)

type Event struct {
	Type EventType
	Node Node
}

type Emitter interface {
	EmitUIEvent(event Event)
}
