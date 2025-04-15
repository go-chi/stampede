package stampede

import (
	"net/http"
	"time"
)

type Options struct {
	TTL       time.Duration // TODO: pkg-level default of 1 minute ..
	SkipCache bool          // defalt false

	HTTPStatusTTL           func(status int) time.Duration
	HTTPCacheKeyRequestBody bool        // TODO: we want this to be default true ..
	HTTPCacheKeyVary        http.Header // default empty ..
}

func WithTTL(ttl time.Duration) Option {
	return func(o *Options) {
		o.TTL = ttl
	}
}

func WithSkipCache(skip bool) Option {
	return func(o *Options) {
		o.SkipCache = skip
	}
}

func WithHTTPStatusTTL(fn func(status int) time.Duration) Option {
	return func(o *Options) {
		o.HTTPStatusTTL = fn
	}
}

func WithHTTPCacheKeyRequestBody(b bool) Option {
	return func(o *Options) {
		o.HTTPCacheKeyRequestBody = b
	}
}

func WithHTTPCacheKeyVary(vary http.Header) Option {
	return func(o *Options) {
		o.HTTPCacheKeyVary = vary
	}
}

type Option func(*Options)
