TODO
====

- [x] Review options.. ie. 
    - [x] WithCacheKeyRequestBody
    - [x] what else..?

- [x] lets add another workspace where we do e2e tests with chi, cachestore-mem, etc.
      this way just go-chi/stampede package can be very lightweight for its go.mod

- [x] vary header support, which will adjust cache key too, and singleflight key.. etc.

- [x] HTTPStatusTTL isn't used anywhere..

- [ ] what if there is a panic in the caller fn or http handelr..?

- [ ] developer experience on per-handler setup..?

- [ ] update examples and README
