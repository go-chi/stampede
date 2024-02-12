module example

go 1.21.7

replace github.com/go-chi/stampede => ../

require (
	github.com/go-chi/chi/v5 v5.0.11
	github.com/go-chi/stampede v0.5.1
)

require (
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/goware/singleflight v0.2.0 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
)
