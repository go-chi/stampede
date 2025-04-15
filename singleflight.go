package stampede

import (
	"log/slog"
	"net/http"
)

func Singleflight(logger *slog.Logger, vary http.Header) func(next http.Handler) http.Handler {

	// TODO: we need the vary stuff...
	// which will set the cache key accordingly..
	handler := Handler2(logger, nil, 0, WithSkipCache(true))

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handler(next).ServeHTTP(w, r)
		})
	}
}
