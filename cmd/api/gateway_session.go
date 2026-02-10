package main

import (
	"path"
	"strings"

	insightifyv1 "insightify/gen/go/insightify/v1"

	"connectrpc.com/connect"
)

func inferRepoName(repoURL string) string {
	repoURL = strings.TrimSpace(repoURL)
	if repoURL == "" {
		return ""
	}
	trimmed := strings.TrimSuffix(repoURL, "/")
	trimmed = strings.TrimSuffix(trimmed, ".git")
	if strings.Contains(trimmed, ":") && !strings.Contains(trimmed, "://") {
		// git@github.com:owner/repo form
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) == 2 {
			trimmed = parts[1]
		}
	}
	name := path.Base(trimmed)
	name = strings.TrimSpace(name)
	if name == "." || name == "/" {
		return ""
	}
	return name
}

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
