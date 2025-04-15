package stampede

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

func Handler(logger *slog.Logger, cacheSize int, ttl time.Duration, paths ...string) func(next http.Handler) http.Handler {
	defaultKeyFunc := func(r *http.Request) (uint64, error) {
		// Read the request payload, and then setup buffer for future reader
		var err error
		var buf []byte
		if r.Body != nil {
			buf, err = io.ReadAll(r.Body)
			if err != nil {
				return 0, err
			}
			r.Body = io.NopCloser(bytes.NewBuffer(buf))
		}

		// Prepare cache key based on request URL path and the request data payload.
		key := BytesToHash([]byte(strings.ToLower(r.URL.Path)), buf)
		return key, nil
	}

	return HandlerWithKey(logger, cacheSize, ttl, defaultKeyFunc, paths...)
}

func HandlerWithKey(logger *slog.Logger, cacheSize int, ttl time.Duration, keyFunc CacheKeyFunc, paths ...string) func(next http.Handler) http.Handler {
	// mapping of url paths that are cacheable by the stampede handler
	pathMap := map[string]struct{}{}
	for _, path := range paths {
		pathMap[strings.ToLower(path)] = struct{}{}
	}

	// Stampede handler with set ttl for how long content is fresh.
	// Requests sent to this handler will be coalesced and in scenarios
	// where there is a "stampede" or parallel requests for the same
	// method and arguments, there will be just a single handler that
	// executes, and the remaining handlers will use the response from
	// the first request. The content thereafter will be cached for up to
	// ttl time for subsequent requests for further caching.
	h := stampedeHandler(logger, cacheSize, ttl, keyFunc)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Cache all paths, as whitelist has not been provided
			if len(pathMap) == 0 {
				h(next).ServeHTTP(w, r)
				return
			}

			// Match specific whitelist of paths
			if _, ok := pathMap[strings.ToLower(r.URL.Path)]; ok {
				// stampede-cache the matching path
				h(next).ServeHTTP(w, r)

			} else {
				// no caching
				next.ServeHTTP(w, r)
			}
		})
	}
}

type CacheKeyFunc func(r *http.Request) (uint64, error)

func stampedeHandler(logger *slog.Logger, cacheSize int, ttl time.Duration, keyFunc CacheKeyFunc) func(next http.Handler) http.Handler {
	cache := NewCacheKV[uint64, responseValue](cacheSize, ttl, ttl*2)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			// cache key for the request
			key, err := keyFunc(r)
			if err != nil {
				logger.Warn("stampede: fail to compute cache key", "err", err)
				next.ServeHTTP(w, r)
				return
			}

			// mark the request that actually processes the response
			first := false

			// process request (single flight) – this will block all subsequent requests
			// until the first request is processed
			cachedVal, err := cache.GetFresh(r.Context(), key, func() (responseValue, error) {
				first = true
				buf := bytes.NewBuffer(nil)
				ww := &responseWriter{ResponseWriter: w, tee: buf}

				next.ServeHTTP(ww, r)

				val := responseValue{
					headers: ww.Header(),
					status:  ww.Status(),
					body:    buf.Bytes(),

					// the handler may not write header and body in some logic,
					// while writing only the body, an attempt is made to write the default header (http.StatusOK)
					skip: !ww.IsValid(),
				}
				return val, nil
			})

			// the first request to trigger the fetch should return as it's already
			// responded to the client
			if first {
				return
			}

			// handle response for subsequent requests
			if err != nil {
				logger.Error("stampede: fail to get value, serving standard request handler", "err", err)
				next.ServeHTTP(w, r)
				return
			}

			// if the handler did not write a header, then serve the next handler
			// a standard request handler
			if cachedVal.skip {
				next.ServeHTTP(w, r)
				return
			}

			// copy headers from the first request to the response writer
			respHeader := w.Header()
			for k, v := range cachedVal.headers {
				// Prevent certain headers to override the current
				// value of that header. This is important when you don't want a
				// header to affect all subsequent requests (for instance, when
				// working with several CORS domains, you don't want the first domain
				// to be recorded an to be printed in all responses)
				headerKey := strings.ToLower(k)
				if strings.HasPrefix(headerKey, "access-control-") {
					continue
				}
				respHeader[k] = v
			}

			w.WriteHeader(cachedVal.status)
			w.Write(cachedVal.body)
		})
	}
}

// responseValue is response payload we will be coalescing
type responseValue struct {
	headers http.Header `json:"headers"`
	status  int         `json:"status"`
	body    []byte      `json:"body"`
	skip    bool        `json:"skip"`
}

type responseWriter struct {
	http.ResponseWriter
	wroteHeader bool
	code        int
	bytes       int
	tee         io.Writer
}

func (b *responseWriter) WriteHeader(code int) {
	if !b.wroteHeader {
		b.code = code
		b.wroteHeader = true
		b.ResponseWriter.WriteHeader(code)
	}
}

func (b *responseWriter) IsValid() bool {
	return b.wroteHeader && (b.code >= 100 && b.code < 999)
}

func (b *responseWriter) Write(buf []byte) (int, error) {
	b.maybeWriteHeader()
	n, err := b.ResponseWriter.Write(buf)
	if b.tee != nil {
		_, err2 := b.tee.Write(buf[:n])
		if err == nil {
			err = err2
		}
	}
	b.bytes += n
	return n, err
}

func (b *responseWriter) maybeWriteHeader() {
	if !b.wroteHeader {
		b.WriteHeader(http.StatusOK)
	}
}

func (b *responseWriter) Status() int {
	return b.code
}

func (b *responseWriter) BytesWritten() int {
	return b.bytes
}
