package stampede_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/stampede"
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

	// Create the fast handler
	// fastHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	// 	mu.Lock()
	// 	callCount++
	// 	mu.Unlock()

	// 	w.WriteHeader(http.StatusOK)
	// 	w.Write([]byte("fast response"))
	// })

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
