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
		stampede.WithHTTPCacheKeyRequestHeaders([]string{"AuthorizatioN"}),
	)

	r.With(cacheMiddleware).Get("/cached", func(w http.ResponseWriter, r *http.Request) {
		// processing..
		time.Sleep(1 * time.Second)

		w.WriteHeader(200)
		w.Write([]byte("...hi"))
	})

	http.ListenAndServe(":3333", r)
}
