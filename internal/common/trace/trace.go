package trace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	HeaderName = "X-Trace-Id"
	QueryName  = "trace_id"
)

type contextKey string

const traceContextKey contextKey = "trace_id"

var traceIDPattern = regexp.MustCompile(`^[a-zA-Z0-9._:-]{8,128}$`)

func NewID() string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "trc_fallback"
	}
	return "trc_" + strconvUnixMilli() + "_" + hex.EncodeToString(raw[:])
}

func Normalize(id string) string {
	v := strings.TrimSpace(id)
	if v == "" {
		return ""
	}
	if !traceIDPattern.MatchString(v) {
		return ""
	}
	return v
}

func ExtractHTTP(r *http.Request) string {
	if r == nil {
		return NewID()
	}
	if v := Normalize(r.Header.Get(HeaderName)); v != "" {
		return v
	}
	if v := Normalize(r.URL.Query().Get(QueryName)); v != "" {
		return v
	}
	return NewID()
}

func InjectHTTPResponse(w http.ResponseWriter, traceID string) {
	if w == nil {
		return
	}
	id := Normalize(traceID)
	if id == "" {
		return
	}
	w.Header().Set(HeaderName, id)
}

func WithContext(ctx context.Context, traceID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	id := Normalize(traceID)
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, traceContextKey, id)
}

func FromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(traceContextKey).(string)
	return Normalize(v)
}

func strconvUnixMilli() string {
	return strings.ReplaceAll(time.Now().UTC().Format("20060102T150405.000"), ".", "")
}
