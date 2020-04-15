# Stampede

Prevents cache stampede https://en.wikipedia.org/wiki/Cache_stampede by only running a single data fetch operation per expired / missing key regardless of number of requests to that key.

## Example:

```go
import (
	"net/http"

	"github.com/goware/stampede"
)

var (
	reqCache = stampede.NewCache(5*time.Second, 10*time.Second)
)

func handler(w http.ResponseWriter, r *http.Request) {	
	data, err := reqCache.Get(r.URL.Path, fetchData, false)	
	if err != nil {	
		w.WriteHeader(503)
		return	
	}

	w.Write(data.([]byte))
}

func fetchData() (interface{}, error) {	
	// fetch from remote source.. or compute/render..
	data := []byte("some response data")

	return data, nil	
}
```
