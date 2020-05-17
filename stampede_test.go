package stampede_test

import (
	"context"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/stampede"
	"github.com/stretchr/testify/assert"
)

func TestGet(t *testing.T) {
	var count uint64
	cache := stampede.NewCache(512, time.Duration(2*time.Second), time.Duration(5*time.Second))

	// repeat test multiple times
	for x := 0; x < 5; x++ {
		// time.Sleep(1 * time.Second)

		var wg sync.WaitGroup
		numGoroutines := runtime.NumGoroutine()

		n := 10
		ctx := context.Background()

		for i := 0; i < n; i++ {
			t.Logf("numGoroutines now %d", runtime.NumGoroutine())

			wg.Add(1)
			go func() {
				defer wg.Done()

				val, err := cache.Get(ctx, "t1", func(ctx context.Context) (interface{}, error) {
					t.Log("cache.Get(t1, ...)")

					// some extensive op..
					time.Sleep(2 * time.Second)
					atomic.AddUint64(&count, 1)

					return "result1", nil
				})

				assert.NoError(t, err)
				assert.Equal(t, "result1", val)
			}()
		}

		wg.Wait()

		// ensure single call
		assert.Equal(t, uint64(1), count)

		// confirm same before/after num of goroutines
		t.Logf("numGoroutines now %d", runtime.NumGoroutine())
		assert.Equal(t, numGoroutines, runtime.NumGoroutine())

	}
}

func TestHandler(t *testing.T) {
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

	h := stampede.Handler(512, 1*time.Second)

	ts := httptest.NewServer(counter(recoverer(h(http.HandlerFunc(app)))))
	defer ts.Close()

	var wg sync.WaitGroup

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Get(ts.URL)
			if err != nil {
				t.Fatal(err)
			}

			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()

			// log.Println("got resp:", resp, "len:", len(body), "body:", string(body))

			if string(body) != string(expectedBody) {
				t.Error("expecting response body:", string(expectedBody))
			}

			if resp.StatusCode != expectedStatus {
				t.Error("expecting response status:", expectedStatus)
			}

			if resp.Header.Get("X-Httpjoin") != "test" {
				t.Error("expecting x-httpjoin test header")
			}

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

func TestHash(t *testing.T) {
	h1 := stampede.BytesToHash([]byte{1, 2, 3})
	assert.Equal(t, uint64(8376154270085342629), h1)

	h2 := stampede.StringToHash("123")
	assert.Equal(t, uint64(4353148100880623749), h2)
}
