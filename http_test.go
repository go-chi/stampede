package stampede_test

import (
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/cors"
	"github.com/go-chi/stampede"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSingleflightHTTPHandler(t *testing.T) {
	// Create a counter to track how many times handlers are called
	var callCount int
	var mu sync.Mutex

	// Create a test mux
	mux := http.NewServeMux()

	// Create the slow handler
	endpoint := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callCount++
		mu.Unlock()

		// Simulate processing time
		time.Sleep(100 * time.Millisecond)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("slow response"))
	})

	// Apply Handler2 to the slow handler only
	wrappedSlowHandler := stampede.Singleflight(slog.Default(), nil)(endpoint)

	// Register the handlers with the mux
	mux.Handle("/slow", wrappedSlowHandler)
	// mux.Handle("/fast", fastHandler) // Fast handler is not wrapped

	// Create a test server with the mux
	server := httptest.NewServer(mux)
	defer server.Close()

	// Test concurrent requests to the slow endpoint
	var wg sync.WaitGroup
	concurrentRequests := 20

	// Reset call count before concurrent test
	mu.Lock()
	callCount = 0
	mu.Unlock()

	for i := 0; i < concurrentRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Get(server.URL + "/slow")
			require.NoError(t, err)
			defer resp.Body.Close()

			_, err = io.ReadAll(resp.Body)
			require.NoError(t, err)
		}()
	}

	wg.Wait()

	// Verify call count - should be 1 if singleflight is working
	require.Equal(t, 1, callCount)
}

func TestHTTPCachingHandler(t *testing.T) {
	// Create a counter to track how many times handlers are called
	var callCount int
	var mu sync.Mutex

	// Create a test mux
	mux := http.NewServeMux()

	// Create the slow handler
	slowHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callCount++
		mu.Unlock()

		// Simulate processing time
		time.Sleep(100 * time.Millisecond)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("slow response"))
	})

	// Apply Handler2 to the slow handler only
	// cache, _ := memcache.NewBackend(1000, cachestore.WithDefaultKeyExpiry(10*time.Second))
	cache := newMockCacheBackend()

	wrappedSlowHandler := stampede.Handler(slog.Default(), cache, 5*time.Second,
		stampede.WithHTTPStatusTTL(func(status int) time.Duration {
			switch {
			case status >= 200 && status < 300:
				return 1 * time.Second
			case status >= 400 && status < 500:
				return 10 * time.Second
			case status == http.StatusNotFound:
				return 30 * time.Second // Special case for 404
			default:
				return 0
			}
		}),
	)(slowHandler)

	// Register the handlers with the mux
	mux.Handle("/slow", wrappedSlowHandler)
	// mux.Handle("/fast", fastHandler) // Fast handler is not wrapped

	// Create a test server with the mux
	server := httptest.NewServer(mux)
	defer server.Close()

	// Test concurrent requests to the slow endpoint
	var wg sync.WaitGroup
	concurrentRequests := 20

	// Reset call count before concurrent test
	mu.Lock()
	callCount = 0
	mu.Unlock()

	for i := 0; i < concurrentRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Get(server.URL + "/slow")
			require.NoError(t, err)
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, "slow response", string(body))
		}()
	}

	wg.Wait()

	// Verify call count - should be 1 if singleflight is working
	require.Equal(t, 1, callCount)
}

func TestHTTPCachingHandler2(t *testing.T) {
	var numRequests = 30

	var hits uint32
	var expectedStatus int = 201
	var expectedBody = []byte("hi")

	app := func(w http.ResponseWriter, r *http.Request) {
		// log.Println("app handler..")

		atomic.AddUint32(&hits, 1)

		hitsNow := atomic.LoadUint32(&hits)
		if hitsNow > 1 {
			// panic("uh oh")
		}

		// time.Sleep(100 * time.Millisecond) // slow handler
		w.Header().Set("X-Httpjoin", "test")
		w.WriteHeader(expectedStatus)
		w.Write(expectedBody)
	}

	var count uint32
	counter := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddUint32(&count, 1)
			next.ServeHTTP(w, r)
			atomic.AddUint32(&count, ^uint32(0))
			// log.Println("COUNT:", atomic.LoadUint32(&count))
		})
	}

	recoverer := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if r := recover(); r != nil {
					log.Println("recovered panicing request:", r)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}

	cache := newMockCacheBackend()
	h := stampede.Handler(slog.Default(), cache, 1*time.Second)

	ts := httptest.NewServer(counter(recoverer(h(http.HandlerFunc(app)))))
	defer ts.Close()

	var wg sync.WaitGroup

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Get(ts.URL)
			if err != nil {
				panic(err)
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				panic(err)
			}
			defer resp.Body.Close()

			// log.Println("got resp:", resp, "len:", len(body), "body:", string(body))

			if string(body) != string(expectedBody) {
				t.Error("expecting response body:", string(expectedBody))
			}

			if resp.StatusCode != expectedStatus {
				t.Error("expecting response status:", expectedStatus)
			}

			assert.Equal(t, "test", resp.Header.Get("X-Httpjoin"), "expecting x-httpjoin test header")
		}()
	}

	wg.Wait()

	totalHits := atomic.LoadUint32(&hits)
	// if totalHits > 1 {
	// 	t.Error("handler was hit more than once. hits:", totalHits)
	// }
	log.Println("total hits:", totalHits)

	finalCount := atomic.LoadUint32(&count)
	if finalCount > 0 {
		t.Error("queue count was expected to be empty, but count:", finalCount)
	}
	log.Println("final count:", finalCount)
}

func TestBypassCORSHeaders(t *testing.T) {
	var expectedStatus int = 200
	var expectedBody = []byte("hi")

	var count uint64

	domains := []string{
		"google.com",
		"sequence.build",
		"horizon.io",
		"github.com",
		"ethereum.org",
	}

	app := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Another-Header", "wakka")
		w.WriteHeader(expectedStatus)
		w.Write(expectedBody)

		atomic.AddUint64(&count, 1)
	}

	cache := newMockCacheBackend()
	h := stampede.Handler(slog.Default(), cache, 5*time.Second)
	c := cors.New(cors.Options{
		AllowedOrigins: domains,
		AllowedMethods: []string{"GET"},
		AllowedHeaders: []string{"*"},
	}).Handler

	ts := httptest.NewServer(c(h(http.HandlerFunc(app))))
	defer ts.Close()

	var mu sync.Mutex

	for i := 0; i < 10; i++ {
		var wg sync.WaitGroup
		var domainsHit = map[string]bool{}

		for _, domain := range domains {
			wg.Add(1)
			go func(domain string) {
				defer wg.Done()

				req, err := http.NewRequest("GET", ts.URL, nil)
				assert.NoError(t, err)
				req.Header.Set("Origin", domain)

				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					panic(err)
				}

				body, err := io.ReadAll(resp.Body)
				if err != nil {
					panic(err)
				}
				defer resp.Body.Close()

				if string(body) != string(expectedBody) {
					t.Error("expecting response body:", string(expectedBody))
				}

				if resp.StatusCode != expectedStatus {
					t.Error("expecting response status:", expectedStatus)
				}

				mu.Lock()
				domainsHit[resp.Header.Get("Access-Control-Allow-Origin")] = true
				mu.Unlock()

				assert.Equal(t, "wakka", resp.Header.Get("X-Another-Header"))
			}(domain)
		}

		wg.Wait()

		// expect all domains to be returned and recorded in domainsHit
		for _, domain := range domains {
			assert.True(t, domainsHit[domain])
		}

		// expect to have only one actual hit
		assert.Equal(t, uint64(1), count)
	}
}

func TestEmptyHandlerFunc(t *testing.T) {
	mux := http.NewServeMux()
	cache := newMockCacheBackend()
	middleware := stampede.Handler(slog.Default(), cache, 1*time.Hour)
	mux.Handle("/", middleware(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		t.Log(r.Method, r.URL)
	})))

	ts := httptest.NewServer(mux)
	defer ts.Close()

	{
		req, err := http.NewRequest(http.MethodGet, ts.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		t.Log(resp.StatusCode)
	}
	{
		req, err := http.NewRequest(http.MethodGet, ts.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		t.Log(resp.StatusCode)
	}
}
