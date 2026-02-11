package llminteraction

import (
	"strings"
	"sync"

	insightifyv1 "insightify/gen/go/insightify/v1"

	"google.golang.org/protobuf/proto"
)

type chatConversation struct {
	mu          sync.RWMutex
	events      []*insightifyv1.ChatEvent
	subscribers map[int]chan *insightifyv1.ChatEvent
	nextSubID   int
	lastSeq     int64
	closed      bool
}

func defaultConversationID(runID string) string {
	return strings.TrimSpace(runID)
}

func cloneChatEvent(ev *insightifyv1.ChatEvent) *insightifyv1.ChatEvent {
	if ev == nil {
		return nil
	}
	cloned, ok := proto.Clone(ev).(*insightifyv1.ChatEvent)
	if !ok {
		return nil
	}
	return cloned
}

func (h *Handler) EnsureConversation(runID, conversationID string) string {
	if h == nil {
		return ""
	}
	runID = strings.TrimSpace(runID)
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		conversationID = defaultConversationID(runID)
	}
	if conversationID == "" {
		return ""
	}

	h.convMu.Lock()
	defer h.convMu.Unlock()

	// Preserve existing history if the same run binds to a new conversation ID.
	if runID != "" {
		if existingID := strings.TrimSpace(h.runToConv[runID]); existingID != "" && existingID != conversationID {
			existingConv := h.conversations[existingID]
			targetConv := h.conversations[conversationID]
			if targetConv == nil {
				targetConv = &chatConversation{
					events:      make([]*insightifyv1.ChatEvent, 0, 128),
					subscribers: make(map[int]chan *insightifyv1.ChatEvent),
				}
				h.conversations[conversationID] = targetConv
			}
			if existingConv != nil && len(existingConv.events) > 0 && len(targetConv.events) == 0 {
				targetConv.events = make([]*insightifyv1.ChatEvent, 0, len(existingConv.events))
				for _, ev := range existingConv.events {
					cloned := cloneChatEvent(ev)
					if cloned == nil {
						continue
					}
					cloned.ConversationId = conversationID
					targetConv.events = append(targetConv.events, cloned)
					if cloned.GetSeq() > targetConv.lastSeq {
						targetConv.lastSeq = cloned.GetSeq()
					}
				}
				targetConv.closed = existingConv.closed
			}
		}
	}

	conv := h.conversations[conversationID]
	if conv == nil {
		conv = &chatConversation{
			events:      make([]*insightifyv1.ChatEvent, 0, 128),
			subscribers: make(map[int]chan *insightifyv1.ChatEvent),
		}
		h.conversations[conversationID] = conv
	}
	if runID != "" {
		h.runToConv[runID] = conversationID
		h.convToRun[conversationID] = runID
	}
	return conversationID
}

func (h *Handler) ConversationIDByRun(runID string) string {
	if h == nil {
		return ""
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return ""
	}
	h.convMu.RLock()
	convID := h.runToConv[runID]
	h.convMu.RUnlock()
	if strings.TrimSpace(convID) != "" {
		return convID
	}
	return defaultConversationID(runID)
}

func (h *Handler) RunIDByConversation(conversationID string) string {
	if h == nil {
		return ""
	}
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return ""
	}
	h.convMu.RLock()
	runID := h.convToRun[conversationID]
	h.convMu.RUnlock()
	return strings.TrimSpace(runID)
}

func (h *Handler) AppendChatEvent(runID, conversationID string, ev *insightifyv1.ChatEvent) {
	if h == nil || ev == nil {
		return
	}
	conversationID = h.EnsureConversation(runID, conversationID)
	if conversationID == "" {
		return
	}

	h.convMu.RLock()
	conv := h.conversations[conversationID]
	h.convMu.RUnlock()
	if conv == nil {
		return
	}

	conv.mu.Lock()
	defer conv.mu.Unlock()

	conv.lastSeq++
	ev.Seq = conv.lastSeq
	ev.ConversationId = conversationID
	stored := cloneChatEvent(ev)
	if stored == nil {
		return
	}
	conv.events = append(conv.events, stored)
	if len(conv.events) > 512 {
		conv.events = append([]*insightifyv1.ChatEvent(nil), conv.events[len(conv.events)-512:]...)
	}
	for _, sub := range conv.subscribers {
		select {
		case sub <- cloneChatEvent(stored):
		default:
		}
	}
	if ev.EventType == insightifyv1.ChatEvent_EVENT_TYPE_COMPLETE || ev.EventType == insightifyv1.ChatEvent_EVENT_TYPE_ERROR {
		conv.closed = true
		for id, sub := range conv.subscribers {
			close(sub)
			delete(conv.subscribers, id)
		}
	}
}

func (h *Handler) SubscribeConversation(conversationID string, fromSeq int64) ([]*insightifyv1.ChatEvent, <-chan *insightifyv1.ChatEvent, func()) {
	if h == nil {
		return nil, nil, nil
	}
	conversationID = h.EnsureConversation("", conversationID)
	if conversationID == "" {
		return nil, nil, nil
	}

	h.convMu.RLock()
	conv := h.conversations[conversationID]
	h.convMu.RUnlock()
	if conv == nil {
		return nil, nil, nil
	}

	conv.mu.Lock()
	defer conv.mu.Unlock()

	snapshot := make([]*insightifyv1.ChatEvent, 0, len(conv.events))
	for _, ev := range conv.events {
		if ev.GetSeq() <= fromSeq {
			continue
		}
		if c := cloneChatEvent(ev); c != nil {
			snapshot = append(snapshot, c)
		}
	}
	if conv.closed {
		return snapshot, nil, func() {}
	}

	subID := conv.nextSubID
	conv.nextSubID++
	ch := make(chan *insightifyv1.ChatEvent, 64)
	conv.subscribers[subID] = ch

	cancel := func() {
		conv.mu.Lock()
		defer conv.mu.Unlock()
		sub, ok := conv.subscribers[subID]
		if !ok {
			return
		}
		delete(conv.subscribers, subID)
		close(sub)
	}

	return snapshot, ch, cancel
}
