package main

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/stampede"
	memcache "github.com/goware/cachestore-mem"
)

func main() {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("index"))
	})

	cache, err := memcache.NewBackend(1000)
	if err != nil {
		panic(err)
	}

	cacheMiddleware := stampede.Handler(
		slog.Default(), cache, 5*time.Second,
		stampede.WithHTTPStatusTTL(func(status int) time.Duration {
			if status == 200 {
				return 10 * time.Second
			} else if status == 404 {
				return 1 * time.Second
			} else if status >= 500 {
				return 0 // no cache
			} else {
				return 0 // no cache
			}
		}),
	)

	r.With(cacheMiddleware).Get("/cached", func(w http.ResponseWriter, r *http.Request) {
		// processing..
		time.Sleep(1 * time.Second)

		if r.URL.Query().Get("error") == "true" {
			w.WriteHeader(500)
			w.Write([]byte("error"))
			return
		}

		if r.URL.Query().Get("notfound") == "true" {
			w.WriteHeader(404)
			w.Write([]byte("notfound"))
			return
		}

		w.WriteHeader(200)
		w.Write([]byte("...hi"))
	})

	http.ListenAndServe(":3333", r)
}
