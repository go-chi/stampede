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

func HandlerWithKey(logger *slog.Logger, cacheBackend cachestore.Backend, ttl time.Duration, cacheKeyFunc CacheKeyFunc, options ...Option) func(next http.Handler) http.Handler {
	opts := getOptions(ttl, options...)

	// Combine various cache key functions into a single cache key value.
	cacheKeyWithRequestHeaders := cacheKeyWithRequestHeaders(opts.HTTPCacheKeyRequestHeaders)

	comboCacheKeyFunc := func(r *http.Request) (uint64, error) {
		var cacheKey1, cacheKey2, cacheKey3, cacheKey4 uint64
		var err error
		cacheKey1, err = cacheKeyWithRequestURL(r)
		if err != nil {
			return 0, err
		}
		if opts.HTTPCacheKeyRequestBody {
			cacheKey2, err = cacheKeyWithRequestBody(r)
			if err != nil {
				return 0, err
			}
		}
		if len(opts.HTTPCacheKeyRequestHeaders) > 0 {
			cacheKey3, err = cacheKeyWithRequestHeaders(r)
			if err != nil {
				return 0, err
			}
		}
		if cacheKeyFunc != nil {
			cacheKey4, err = cacheKeyFunc(r)
			if err != nil {
				return 0, err
			}
		}
		return cacheKey1 + cacheKey2 + cacheKey3 + cacheKey4, nil
	}

	var cache cachestore.Store[responseValue]
	if cacheBackend != nil {
		cache = cachestore.OpenStore[responseValue](cacheBackend)
	}
	h := stampedeHandler(logger, cache, comboCacheKeyFunc, opts)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h(next).ServeHTTP(w, r)
		})
	}
}

func Handler(logger *slog.Logger, cacheBackend cachestore.Backend, ttl time.Duration, options ...Option) func(next http.Handler) http.Handler {
	return HandlerWithKey(logger, cacheBackend, ttl, nil, options...)
}

func cacheKeyWithRequestURL(r *http.Request) (uint64, error) {
	return StringToHash(strings.ToLower(r.URL.Path)), nil
}

func cacheKeyWithRequestBody(r *http.Request) (uint64, error) {
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

	// Prepare cache key based on the request data payload.
	return BytesToHash(buf), nil
}

func cacheKeyWithRequestHeaders(headers []string) func(r *http.Request) (uint64, error) {
	return func(r *http.Request) (uint64, error) {
		if len(headers) == 0 {
			return 0, nil
		}
		var keys []string
		for _, header := range headers {
			v := r.Header.Get(header)
			if v == "" {
				continue
			}
			keys = append(keys, fmt.Sprintf("%s:%s", strings.ToLower(header), v))
		}
		return StringToHash(keys...), nil
	}
}

type CacheKeyFunc func(r *http.Request) (uint64, error)

func stampedeHandler(logger *slog.Logger, cache cachestore.Store[responseValue], cacheKeyFunc CacheKeyFunc, options *Options) func(next http.Handler) http.Handler {
	stampede := NewStampede(cache)
	stampede.SetOptions(options)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cacheKey, err := cacheKeyFunc(r)
			if err != nil {
				logger.Warn("stampede: fail to compute cache key", "err", err)
				next.ServeHTTP(w, r)
				return
			}

			firstRequest := false

			cachedVal, err := stampede.Do(fmt.Sprintf("%d", cacheKey), func() (responseValue, error) {
				firstRequest = true
				buf := bytes.NewBuffer(nil)
				ww := &responseWriter{ResponseWriter: w, tee: buf}

				next.ServeHTTP(ww, r)

				val := responseValue{
					Headers: ww.Header(),
					Status:  ww.Status(),
					Body:    buf.Bytes(),

					// the handler may not write header and body in some logic,
					// while writing only the body, an attempt is made to write the default header (http.StatusOK)
					Skip: !ww.IsValid(),
				}
				return val, nil
			})

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
			respHeader.Set("x-cache", "hit") // TODO: confirm works....

			w.WriteHeader(cachedVal.Status)
			w.Write(cachedVal.Body)
		})
	}
}

type responseValue struct {
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
