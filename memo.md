m-mainline
x-external
a-algorithm
b-build
c-codebase

go run ./cmd/archflow --repo  --phase c1 --provider gemini --model gemini-2.5-pro

---

Auth/Security TODO (2026-02-07)
- Current state: InitRun sets HttpOnly session cookie (`insightify_session_id`).
- Current state: StartRun accepts session ID from request field or cookie.
- Current state: Session is server-side and bound 1:1 with RunContext.
- Next auth step: Add real user authentication (login + server-verified identity).
- Next auth step: Bind session owner (`user_id`) and enforce owner check on StartRun/WatchRun.
- Next auth step: Replace permissive CORS origin reflection with allowlist.
- Next auth step: Set cookie `Secure=true` in TLS environments and consider `SameSite=None` only when required.
- Next auth step: Add CSRF protection if cross-site credentials are allowed.
