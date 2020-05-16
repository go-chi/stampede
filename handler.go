package stampede

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func Handler(ttl time.Duration) func(next http.Handler) http.Handler {
	cache := NewCache(ttl, ttl*2)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			// cache key for the request
			key := fmt.Sprintf("%s %s", r.Method, strings.ToLower(r.URL.Path))

			// mark the request that actually processes the response
			first := false

			// process request (single flight)
			val, err := cache.GetFresh(r.Context(), key, func(ctx context.Context) (interface{}, error) {
				first = true
				buf := bytes.NewBuffer(nil)
				ww := &responseWriter{ResponseWriter: w, tee: buf}

				next.ServeHTTP(ww, r)

				val := responseValue{
					headers: ww.Header(),
					status:  ww.Status(),
					body:    buf.Bytes(),
					huh:     string(buf.Bytes()),
					time:    time.Now().Unix(),
				}
				return val, nil
			})

			// the first request to trigger the fetch should return as it's already
			// responded to the client
			if first {
				return
			}

			// handle response for other listeners
			if err != nil {
				panic(fmt.Sprintf("stampede: fail to get value, %v", err))
			}

			resp, ok := val.(responseValue)
			if !ok {
				panic("stampede: handler received unexpected response value type")
			}

			for k, v := range resp.headers {
				w.Header().Set(k, strings.Join(v, ", "))
			}

			w.WriteHeader(resp.status)
			w.Write(resp.body)
		})
	}
}

// responseValue is response payload we will be coalescing
type responseValue struct {
	headers http.Header
	status  int
	body    []byte
	huh     string
	time    int64
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
