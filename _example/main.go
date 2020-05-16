package main

import (
	"net/http"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/goware/stampede"
)

func main() {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("index"))
	})

	cached := stampede.Handler(1 * time.Second)

	r.With(cached).Get("/cached", func(w http.ResponseWriter, r *http.Request) {
		// processing..
		time.Sleep(1 * time.Second)

		w.WriteHeader(200)
		w.Write([]byte("...hi"))
	})

	http.ListenAndServe(":3333", r)
}
