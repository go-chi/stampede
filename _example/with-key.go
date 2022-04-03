package main

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/stampede"
)

/**
Example 1: Make two parallel requests:
	First request in first client:
		GET http://localhost:3333/me
		Authorization: Bar

	Second request in second client:
		GET http://localhost:3333/me
		Authorization: Bar

	-> Result of both queries in one time:
			HTTP/1.1 200 OK
			Content-Length: 14
			Content-Type: text/plain; charset=utf-8

			Bearer BarTone

			Response code: 200 (OK); Time: 1ms; Content length: 14 bytes

---------------------------------------------------------------

Example 2: Make two parallel requests:
	First request in first client:
		GET http://localhost:3333/me
		Authorization: Bar

	Second request in second client:
		GET http://localhost:3333/me
		Authorization: Foo

	-> Result of first:
			HTTP/1.1 200 OK
			Content-Length: 14
			Content-Type: text/plain; charset=utf-8

			Bearer Bar

			Response code: 200 (OK); Time: 1ms; Content length: 14 bytes

	-> Result of second:
			HTTP/1.1 200 OK
			Content-Length: 14
			Content-Type: text/plain; charset=utf-8

			Bearer Foo

			Response code: 200 (OK); Time: 1ms; Content length: 14 bytes
*/
func main() {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("index"))
	})

	// Include anything sensitive or user specific, e.g. Authorization Token
	customKeyFunc := func(r *http.Request) uint64 {
		token := r.Header.Get("Authorization")
		return stampede.StringToHash(r.Method, strings.ToLower(strings.ToLower(token)))
	}
	cached := stampede.HandlerWithKey(512, 1*time.Second, customKeyFunc)

	r.With(cached).Get("/me", func(w http.ResponseWriter, r *http.Request) {
		// processing..
		time.Sleep(3 * time.Second)

		w.WriteHeader(200)
		w.Write([]byte(r.Header.Get("Authorization")))
	})

	http.ListenAndServe(":3333", r)
}
