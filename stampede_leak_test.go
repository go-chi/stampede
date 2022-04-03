package stampede_test

import (
	"fmt"
	"github.com/go-chi/stampede"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestMemoryLeak(t *testing.T) {
	mux := http.NewServeMux()
	respRec := httptest.NewRecorder()

	cacheTime := 2 * time.Second
	cache := stampede.NewCache(100, cacheTime, 2*cacheTime)

	s := stampede.NewM()
	s.SetCache(cache)
	h := s.Handler(100, cacheTime)

	n := 10
	for i := 1; i <= n; i++ {
		mux.Handle("/cached-"+strconv.Itoa(i), h(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			rw.Write([]byte(fmt.Sprintf("%v %v \n", "Handled route: ", r.URL.Path)))
		})))
	}

	ts := httptest.NewServer(mux)
	defer ts.Close()

	count := 0
	for i := 1; i <= n; i++ {
		{
			url := "/cached-" + strconv.Itoa(i)
			req, err := http.NewRequest(http.MethodGet, url, nil)
			if err != nil {
				t.Fatal(err)
			}

			mux.ServeHTTP(respRec, req)
			count = count + 1
		}
	}

	assert.Equal(t, 10, count, "count of call")

	time.Sleep(cacheTime * 3)
	leakCnt := cache.Flush().Len()

	assert.Equal(t, 0, leakCnt, "leaked values")
}
