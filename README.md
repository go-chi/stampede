# Stampede

![](https://github.com/go-chi/stampede/workflows/build/badge.svg?branch=master)

Prevents cache stampede https://en.wikipedia.org/wiki/Cache_stampede by only running a
single data fetch operation per expired / missing key regardless of number of requests to that key.


## Example: HTTP Middleware

```go
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
```


## Notes

* Requests passed through the stampede handler will be batched into a single request
when there are parallel requests for the same endpoint/resource. This is also known
as request coalescing.
* Parallel requests for the same endpoint / resource, will be just a single handler call
and the remaining requests will receive the response of the first request handler.
* The response payload for the endpoint / resource will then be cached for up to `ttl`
time duration for subequence requests, which offers further caching. You may also
use a `ttl` value of 0 if you want the response to be as fresh as possible, and still
prevent a stampede scenario on your handler.
* *Security note:* response headers will be the same for all requests, so make sure
to not include anything sensitive or user specific. In the case you require user-specific
stampede handlers, make sure you pass a custom `keyFunc` to the `stampede.Handler` and
split the cache by an account's id. NOTE: we do avoid caching response headers
for CORS, set-cookie and x-ratelimit.

See [example](_example/with_key.go) for a variety of examples.


## LICENSE

MIT
