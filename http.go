package stampede

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	cachestore "github.com/goware/cachestore2"
)

// TODO: we want TTL for OK and TTL for error ..
// but really, it could be any TTL depending on the request ..

// kinda need like ... for status set the TTL ..?

func Handler2(logger *slog.Logger, cacheBackend cachestore.Backend, ttl time.Duration, options ...Option) func(next http.Handler) http.Handler {
	opts := &Options{
		TTL: ttl,
	}
	for _, o := range options {
		o(opts)
	}

	var cacheKeyFunc func(r *http.Request) (uint64, error)

	if !opts.HTTPCacheKeyRequestBody {
		cacheKeyFunc = func(r *http.Request) (uint64, error) {
			return StringToHash(strings.ToLower(r.URL.Path)), nil
		}
	} else {
		cacheKeyFunc = func(r *http.Request) (uint64, error) {
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
	}

	var cache cachestore.Store[responseValue2]
	if cacheBackend != nil {
		cache = cachestore.OpenStore[responseValue2](cacheBackend)
	}
	h := stampedeHandler2(logger, cache, cacheKeyFunc, opts)

	return func(next http.Handler) http.Handler {

		// TODO: the "wee" function(request) doesn't make sense, because
		// we actually need the response ..
		// and also, might want to "vary" on the response headers ....?

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h(next).ServeHTTP(w, r)
		})
	}
}

type CacheKeyFunc func(r *http.Request) (uint64, error)

func stampedeHandler2(logger *slog.Logger, cache cachestore.Store[responseValue2], cacheKeyFunc CacheKeyFunc, options *Options) func(next http.Handler) http.Handler {
	stampede := NewStampede(cache)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			cacheKey, err := cacheKeyFunc(r)
			if err != nil {
				logger.Warn("stampede: fail to compute cache key", "err", err)
				next.ServeHTTP(w, r)
				return
			}

			firstRequest := false

			k := fmt.Sprintf("%d", cacheKey) // TODO ...

			ttl := options.TTL
			_ = ttl

			// TODO: pass down the greater TTL to the .Do()..

			// TODO: .. THEN .. we can check cachedVal.TS and we'll

			cachedVal, err := stampede.Do(k, func() (responseValue2, error) {
				firstRequest = true
				buf := bytes.NewBuffer(nil)
				ww := &responseWriter{ResponseWriter: w, tee: buf}

				next.ServeHTTP(ww, r)

				val := responseValue2{
					Headers: ww.Header(),
					Status:  ww.Status(),
					Body:    buf.Bytes(),

					// the handler may not write header and body in some logic,
					// while writing only the body, an attempt is made to write the default header (http.StatusOK)
					Skip: !ww.IsValid(),
				}
				return val, nil
			}) //, options) // TODO .. we need this..
			_ = cacheKey
			_ = firstRequest

			if firstRequest {
				fmt.Println("first request")
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
			if cachedVal.Skip {
				panic("TODO")
				next.ServeHTTP(w, r)
				return
			}

			// copy headers from the first request to the response writer
			respHeader := w.Header()
			for k, v := range cachedVal.Headers {
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
			respHeader.Set("x-cache", "hit") // TODO: confirm works..

			w.WriteHeader(cachedVal.Status)
			w.Write(cachedVal.Body)
		})
	}
}

type responseValue2 struct {
	Headers http.Header `json:"headers"`
	Status  int         `json:"status"`
	Body    []byte      `json:"body"`
	Skip    bool        `json:"skip"`
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
