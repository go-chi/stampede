module example

go 1.23.0

toolchain go1.24.2

replace github.com/go-chi/stampede => ../

require (
	github.com/go-chi/chi/v5 v5.0.11
	github.com/go-chi/stampede v0.5.1
)

require (
	github.com/goware/cachestore2 v0.12.2 // indirect
	github.com/goware/singleflight v0.3.0 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/klauspost/cpuid/v2 v2.2.10 // indirect
	github.com/zeebo/xxh3 v1.0.2 // indirect
	golang.org/x/sys v0.32.0 // indirect
)
