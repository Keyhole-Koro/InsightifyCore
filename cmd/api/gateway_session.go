package main

import (
	"strings"

	insightifyv1 "insightify/gen/go/insightify/v1"

	"connectrpc.com/connect"
)

func resolveSessionID(req *connect.Request[insightifyv1.StartRunRequest]) string {
	if req == nil {
		return ""
	}
	if sid := strings.TrimSpace(req.Msg.GetSessionId()); sid != "" {
		return sid
	}
	return resolveSessionIDFromCookieHeader(req.Header().Get("Cookie"))
}

func resolveSessionIDFromCookieHeader(cookieHeader string) string {
	if cookieHeader == "" {
		return ""
	}
	for _, part := range strings.Split(cookieHeader, ";") {
		p := strings.TrimSpace(part)
		if !strings.HasPrefix(p, sessionCookieName+"=") {
			continue
		}
		return strings.TrimSpace(strings.TrimPrefix(p, sessionCookieName+"="))
	}
	return ""
}
