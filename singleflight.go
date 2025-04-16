package stampede

import (
	"log/slog"
	"net/http"
)

func Singleflight(logger *slog.Logger, varyHeaders []string) func(next http.Handler) http.Handler {
	handler := Handler(logger, nil, 0, WithSkipCache(true), WithHTTPCacheKeyRequestHeaders(varyHeaders))

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handler(next).ServeHTTP(w, r)
		})
	}
}
