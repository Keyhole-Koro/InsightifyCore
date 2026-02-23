package middleware

import (
	"net/http"

	traceutil "insightify/internal/common/trace"
)

func Trace(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := traceutil.ExtractHTTP(r)
		traceutil.InjectHTTPResponse(w, traceID)
		ctx := traceutil.WithContext(r.Context(), traceID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
