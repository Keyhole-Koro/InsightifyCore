package runner

import (
	"context"
)

// RunEventType matches the proto enum for event types.
type RunEventType int

const (
	EventTypeUnspecified RunEventType = iota
	EventTypeLog
	EventTypeProgress
	EventTypeLLMChunk // New: partial LLM response
	EventTypeComplete
	EventTypeError
)

// RunEvent represents a streamable event from a pipeline worker.
type RunEvent struct {
	Type     RunEventType
	Message  string
	Progress int32  // 0-100
	Chunk    string // For LLM streaming chunks
	Worker   string // Current worker name
}

// RunEventEmitter allows workers to emit events during execution.
type RunEventEmitter interface {
	Emit(event RunEvent)
	EmitLog(message string)
	EmitProgress(percent int32, message string)
	EmitLLMChunk(chunk string)
}

type emitterKey struct{}

// WithEmitter attaches an emitter to the context.
func WithEmitter(ctx context.Context, emitter RunEventEmitter) context.Context {
	return context.WithValue(ctx, emitterKey{}, emitter)
}

// EmitterFrom retrieves the emitter from context, or returns a no-op emitter.
func EmitterFrom(ctx context.Context) RunEventEmitter {
	if e, ok := ctx.Value(emitterKey{}).(RunEventEmitter); ok {
		return e
	}
	return noopEmitter{}
}

// noopEmitter discards all events.
type noopEmitter struct{}

func (noopEmitter) Emit(RunEvent)              {}
func (noopEmitter) EmitLog(string)             {}
func (noopEmitter) EmitProgress(int32, string) {}
func (noopEmitter) EmitLLMChunk(string)        {}

// ChannelEmitter sends events to a channel.
type ChannelEmitter struct {
	Ch     chan<- RunEvent
	Worker string
}

func (e *ChannelEmitter) Emit(event RunEvent) {
	event.Worker = e.Worker
	select {
	case e.Ch <- event:
	default: // non-blocking
	}
}

func (e *ChannelEmitter) EmitLog(message string) {
	e.Emit(RunEvent{Type: EventTypeLog, Message: message})
}

func (e *ChannelEmitter) EmitProgress(percent int32, message string) {
	e.Emit(RunEvent{Type: EventTypeProgress, Progress: percent, Message: message})
}

func (e *ChannelEmitter) EmitLLMChunk(chunk string) {
	e.Emit(RunEvent{Type: EventTypeLLMChunk, Chunk: chunk})
}
