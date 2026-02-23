package ws

import (
	"context"
	"net/http"
	"strings"
	"time"

	insightifyv1 "insightify/gen/go/insightify/v1"
	logctx "insightify/internal/common/logctx"
	traceutil "insightify/internal/common/trace"
	userinteraction "insightify/internal/gateway/service/userinteraction"

	"github.com/gorilla/websocket"
)

// UserInteractionHandler serves interaction endpoints.
// The legacy RPC handlers were removed; websocket handler is used.
type UserInteractionHandler struct {
	svc *userinteraction.Service
}

func NewUserInteractionHandler(svc *userinteraction.Service) *UserInteractionHandler {
	return &UserInteractionHandler{svc: svc}
}

const (
	interactionWSWriteWait = 10 * time.Second
	interactionWSPongWait  = 60 * time.Second
	interactionWSPingEvery = (interactionWSPongWait * 9) / 10
)

var interactionWSUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(_ *http.Request) bool {
		return true
	},
}

type interactionWSInbound struct {
	Type          string `json:"type"`
	RunID         string `json:"runId,omitempty"`
	InteractionID string `json:"interactionId,omitempty"`
	Input         string `json:"input,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

type interactionWSOutbound struct {
	Type             string `json:"type"`
	RunID            string `json:"runId,omitempty"`
	TraceID          string `json:"traceId,omitempty"`
	InteractionID    string `json:"interactionId,omitempty"`
	Waiting          bool   `json:"waiting,omitempty"`
	Closed           bool   `json:"closed,omitempty"`
	Accepted         bool   `json:"accepted,omitempty"`
	AssistantMessage string `json:"assistantMessage,omitempty"`
	Code             string `json:"code,omitempty"`
	Message          string `json:"message,omitempty"`
}

func (h *UserInteractionHandler) HandleInteractionWS(w http.ResponseWriter, r *http.Request) {
	runID := strings.TrimSpace(r.URL.Query().Get("run_id"))
	if runID == "" {
		http.Error(w, "run_id is required", http.StatusBadRequest)
		return
	}
	traceID := traceutil.ExtractHTTP(r)
	ctxWithTrace := traceutil.WithContext(r.Context(), traceID)
	traceutil.InjectHTTPResponse(w, traceID)

	conn, err := interactionWSUpgrader.Upgrade(w, r, nil)
	if err != nil {
		logctx.Error(ctxWithTrace, "interaction ws upgrade failed", err, "run_id", runID)
		return
	}
	defer conn.Close()
	logctx.Info(ctxWithTrace, "interaction ws connected", "run_id", runID)

	ctx, cancel := context.WithCancel(ctxWithTrace)
	defer cancel()

	if err := conn.SetReadDeadline(time.Now().Add(interactionWSPongWait)); err != nil {
		logctx.Error(ctx, "interaction ws set read deadline failed", err, "run_id", runID)
		return
	}
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(interactionWSPongWait))
	})

	writeCh := make(chan interactionWSOutbound, 32)
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		ticker := time.NewTicker(interactionWSPingEvery)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case out := <-writeCh:
				if err := conn.SetWriteDeadline(time.Now().Add(interactionWSWriteWait)); err != nil {
					return
				}
				if err := conn.WriteJSON(out); err != nil {
					return
				}
			case <-ticker.C:
				if err := conn.SetWriteDeadline(time.Now().Add(interactionWSWriteWait)); err != nil {
					return
				}
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()

	subCh, subErr := h.svc.Subscribe(ctx, runID)
	if subErr != nil {
		pushInteractionWS(writeCh, interactionWSOutbound{
			Type:    "error",
			TraceID: traceID,
			Code:    "invalid_argument",
			Message: subErr.Error(),
		})
		cancel()
		<-writerDone
		return
	}

	pushInteractionWS(writeCh, interactionWSOutbound{
		Type:    "subscribed",
		RunID:   runID,
		TraceID: traceID,
	})

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-subCh:
				if !ok {
					return
				}
				switch evt.Kind {
				case userinteraction.SubscriptionEventWaitState:
					state := evt.WaitState
					if state == nil {
						continue
					}
					pushInteractionWS(writeCh, interactionWSOutbound{
						Type:          "wait_state",
						RunID:         runID,
						TraceID:       traceID,
						InteractionID: state.GetInteractionId(),
						Waiting:       state.GetWaiting(),
						Closed:        state.GetClosed(),
					})
				case userinteraction.SubscriptionEventAssistantMessage:
					pushInteractionWS(writeCh, interactionWSOutbound{
						Type:             "assistant_message",
						RunID:            runID,
						TraceID:          traceID,
						InteractionID:    strings.TrimSpace(evt.InteractionID),
						AssistantMessage: strings.TrimSpace(evt.AssistantMessage),
					})
				}
			}
		}
	}()

	for {
		var in interactionWSInbound
		if err := conn.ReadJSON(&in); err != nil {
			cancel()
			<-writerDone
			return
		}
		msgType := strings.ToLower(strings.TrimSpace(in.Type))
		if msgType == "" {
			pushInteractionWS(writeCh, interactionWSOutbound{
				Type:    "error",
				TraceID: traceID,
				Code:    "invalid_argument",
				Message: "type is required",
			})
			continue
		}
		msgRunID := runID
		if v := strings.TrimSpace(in.RunID); v != "" {
			msgRunID = v
		}
		if msgRunID != runID {
			pushInteractionWS(writeCh, interactionWSOutbound{
				Type:    "error",
				TraceID: traceID,
				Code:    "invalid_argument",
				Message: "runId mismatch",
			})
			continue
		}

		switch msgType {
		case "ping":
			pushInteractionWS(writeCh, interactionWSOutbound{Type: "pong", TraceID: traceID})
		case "send":
			out, sendErr := h.svc.Send(ctx, &insightifyv1.SendRequest{
				RunId:         runID,
				InteractionId: strings.TrimSpace(in.InteractionID),
				Input:         strings.TrimSpace(in.Input),
			})
			if sendErr != nil {
				logctx.Error(ctx, "interaction send failed", sendErr, "run_id", runID)
				pushInteractionWS(writeCh, interactionWSOutbound{
					Type:    "error",
					TraceID: traceID,
					Code:    "internal",
					Message: sendErr.Error(),
				})
				continue
			}
			pushInteractionWS(writeCh, interactionWSOutbound{
				Type:             "send_ack",
				RunID:            runID,
				TraceID:          traceID,
				InteractionID:    out.GetInteractionId(),
				Accepted:         out.GetAccepted(),
				AssistantMessage: out.GetAssistantMessage(),
			})
		case "close":
			out, closeErr := h.svc.Close(ctx, &insightifyv1.CloseRequest{
				RunId:         runID,
				InteractionId: strings.TrimSpace(in.InteractionID),
				Reason:        strings.TrimSpace(in.Reason),
			})
			if closeErr != nil {
				logctx.Error(ctx, "interaction close failed", closeErr, "run_id", runID)
				pushInteractionWS(writeCh, interactionWSOutbound{
					Type:    "error",
					TraceID: traceID,
					Code:    "internal",
					Message: closeErr.Error(),
				})
				continue
			}
			pushInteractionWS(writeCh, interactionWSOutbound{
				Type:    "close_ack",
				RunID:   runID,
				TraceID: traceID,
				Closed:  out.GetClosed(),
			})
		default:
			pushInteractionWS(writeCh, interactionWSOutbound{
				Type:    "error",
				TraceID: traceID,
				Code:    "invalid_argument",
				Message: "unsupported type: " + msgType,
			})
		}
	}
}

func pushInteractionWS(writeCh chan interactionWSOutbound, out interactionWSOutbound) {
	if writeCh == nil {
		return
	}
	select {
	case writeCh <- out:
		return
	default:
	}
	select {
	case <-writeCh:
	default:
	}
	select {
	case writeCh <- out:
	default:
	}
}
