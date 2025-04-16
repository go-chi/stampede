package stampede

import (
	"log/slog"
	"net/http"
)

func Singleflight(logger *slog.Logger, varyRequestHeaders []string) func(next http.Handler) http.Handler {
	handler := Handler(logger, nil, 0, WithSkipCache(true), WithHTTPCacheKeyRequestHeaders(varyRequestHeaders))

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handler(next).ServeHTTP(w, r)
		})
	}
}
