package stampede

import (
	"time"
)

type Options struct {
	// TTL is the time-to-live for the cache. NOTE: if this is not set,
	// then we use the package-level default of 1 minute. You can override
	// by passing `WithTTL(time.Second * 10)` to stampede.Do(), or passing
	// `WithSkipCache(true)` to skip the cache entirely.
	//
	// Default: 1 minute
	TTL time.Duration

	// SkipCache is a flag that determines whether the cache should be skipped.
	// If true, the cache will not be used, but the request will still use
	// singleflight request coalescing.
	//
	// Default: false
	SkipCache bool

	// HTTPCacheKeyRequestBody is a flag that determines whether the request body
	// should be used to generate the cache key. This is useful for varying the cache
	// key based on request headers.
	//
	// Default: true
	HTTPCacheKeyRequestBody bool

	// HTTPCacheKeyRequestHeaders is a list of headers that will be used to generate
	// the cache key. This ensures we use the request body contents so we can properly
	// cache different requests that have the same URL but different query params or
	// body content.
	//
	// Default: []
	HTTPCacheKeyRequestHeaders []string

	// HTTPStatusTTL is a function that returns the time-to-live for a given HTTP
	// status code. This allows you to customize the TTL for different HTTP status codes.
	//
	// Default: nil
	HTTPStatusTTL func(status int) time.Duration
}

// WithTTL sets the TTL for the cache.
//
// Default: 1 minute
func WithTTL(ttl time.Duration) Option {
	return func(o *Options) {
		o.TTL = ttl
	}
}

// WithSkipCache sets the SkipCache flag. If true, the cache will not be used,
// but the request will still use singleflight request coalescing.
//
// Default: false
func WithSkipCache(skip bool) Option {
	return func(o *Options) {
		o.SkipCache = skip
	}
}

// WithHTTPStatusTTL sets the HTTPStatusTTL function. This allows you to
// customize the TTL for different HTTP status codes.
//
// Default: nil
func WithHTTPStatusTTL(fn func(status int) time.Duration) Option {
	return func(o *Options) {
		o.HTTPStatusTTL = fn
	}
}

// WithHTTPCacheKeyRequestBody sets the HTTPCacheKeyRequestBody flag. This
// ensures we use the request body contents so we can properly cache different
// requests that have the same URL but different query params or body content.
//
// Default: true
func WithHTTPCacheKeyRequestBody(b bool) Option {
	return func(o *Options) {
		o.HTTPCacheKeyRequestBody = b
	}
}

// WithHTTPCacheKeyRequestHeaders sets the HTTPCacheKeyRequestHeaders list.
// This is useful for varying the cachekey based on request headers.
//
// Default: []
func WithHTTPCacheKeyRequestHeaders(headers []string) Option {
	return func(o *Options) {
		o.HTTPCacheKeyRequestHeaders = headers
	}
}

type Option func(*Options)

// getOptions returns a new Options with the given ttl and options,
// and also applies default values for any options that are not set.
func getOptions(ttl time.Duration, options ...Option) *Options {
	if ttl == 0 {
		ttl = DefaultCacheTTL
	}
	opts := &Options{
		TTL:                        ttl,
		SkipCache:                  false,
		HTTPStatusTTL:              nil,
		HTTPCacheKeyRequestHeaders: nil,
		HTTPCacheKeyRequestBody:    true,
	}
	for _, o := range options {
		o(opts)
	}
	return opts
}
