module example

go 1.22

toolchain go1.24.1

replace github.com/go-chi/stampede => ../

require (
	github.com/go-chi/chi/v5 v5.0.11
	github.com/go-chi/stampede v0.5.1
)

require (
	github.com/goware/singleflight v0.2.0 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/klauspost/cpuid/v2 v2.2.10 // indirect
	github.com/zeebo/xxh3 v1.0.2 // indirect
	golang.org/x/sys v0.30.0 // indirect
)
