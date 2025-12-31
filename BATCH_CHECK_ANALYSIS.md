# OpenFGA Batch Check - Bottleneck Analysis and Recommendations

## Executive Summary

The BatchCheck API in OpenFGA provides efficient batch authorization checking by executing multiple permission checks in a single request. After reviewing the implementation, I've identified several performance bottlenecks and optimization opportunities.

---

## Architecture Overview

### Request Flow

```
BatchCheck Request
    ↓
Server.BatchCheck() [pkg/server/batch_check.go:25]
    ├─ Validate request & resolve authorization model
    ├─ Build check resolver chain
    └─ Execute BatchCheckCommand
          ↓
BatchCheckQuery.Execute() [pkg/server/commands/batch_check_command.go:131]
    ├─ Deduplicate checks by cache key (xxhash)
    ├─ Execute unique checks in parallel via goroutine pool
    ├─ Each check → CheckQuery.Execute() → checkResolver.ResolveCheck()
    └─ Map results back to correlation IDs
```

### Key Components

| Component | File | Role |
|-----------|------|------|
| BatchCheck Handler | `pkg/server/batch_check.go` | HTTP/gRPC endpoint |
| BatchCheckCommand | `pkg/server/commands/batch_check_command.go` | Orchestration logic |
| CheckCommand | `pkg/server/commands/check_command.go` | Single check execution |
| CachedCheckResolver | `internal/graph/cached_resolver.go` | Check response caching |
| LocalChecker | `internal/graph/check.go` | Graph traversal logic |
| CacheController | `internal/cachecontroller/cache_controller.go` | Cache invalidation |
| BoundedTupleReader | `pkg/storage/storagewrappers/bounded_datastore.go` | Datastore throttling |

---

## Identified Bottlenecks

### 1. **Synchronous Pool.Wait() Blocking** (High Impact)

**Location:** `batch_check_command.go:234`

```go
_ = pool.Wait()  // Blocks until ALL checks complete
```

**Problem:** The handler thread blocks until the slowest check in the batch completes. This means:
- Batch latency = max(individual check latencies)
- A single slow check (deep graph traversal, cache miss) delays the entire response
- No partial result streaming

**Impact:** High - Under load, a few slow checks can significantly increase p99 latency for the entire batch.

---

### 2. **Per-Check Object Allocation Overhead** (Medium Impact)

**Location:** `batch_check_command.go:188-199`

```go
pool.Go(func(ctx context.Context) error {
    checkQuery := NewCheckCommand(  // New allocation per check
        bq.datastore,
        bq.checkResolver,
        bq.typesys,
        WithCheckCommandLogger(bq.logger),
        WithCheckCommandCache(bq.sharedCheckResources, bq.cacheSettings),
        // ...
    )
    // ...
})
```

**Problem:** For each unique check in the batch, a new `CheckQuery` struct is allocated with multiple options applied. While small, this creates GC pressure for large batches.

**Impact:** Medium - Noticeable at high throughput (thousands of requests/sec).

---

### 3. **Deduplication Hash Computation** (Low-Medium Impact)

**Location:** `batch_check_command.go:150-166`

```go
for _, check := range params.Checks {
    key, err := generateCacheKeyFromCheck(check, params.StoreID, bq.typesys.GetAuthorizationModelID())
    // Hash computation for every check
}
```

**Problem:** Every check in the batch requires xxhash computation, including:
- Serializing contextual tuples
- Serializing context (structpb)
- Computing 64-bit hash

For batches with mostly unique checks, the deduplication overhead may exceed its benefit.

**Impact:** Low-Medium - Profiling needed to quantify actual overhead vs. deduplication savings.

---

### 4. **sync.Map Overhead in Result Collection** (Low Impact)

**Location:** `batch_check_command.go:168, 211, 240`

```go
var resultMap = new(sync.Map)
// ...
resultMap.Store(key, &BatchCheckOutcome{...})
// ...
res, _ := resultMap.Load(cacheKey)
```

**Problem:** `sync.Map` is optimized for read-heavy workloads with few distinct keys. The batch check pattern (write once, read once per key) doesn't match this profile. A pre-sized `map` with mutex could be more efficient.

**Impact:** Low - Minor overhead, but adds up at scale.

---

### 5. **Cache Invalidation Latency** (Medium Impact)

**Location:** `cache_controller.go:207-226`

```go
func (c *InMemoryCacheController) InvalidateIfNeeded(ctx context.Context, storeID string) {
    _, present := c.inflightInvalidations.LoadOrStore(storeID, struct{}{})
    if present {
        return  // Another invalidation in progress
    }

    go func() {
        c.findChangesAndInvalidateIfNecessary(ctx, storeID)
        // ...
    }()
}
```

**Problem:**
- Cache invalidation is async with 1-second timeout (`cache_controller.go:260`)
- Only one invalidation can run per store at a time
- First check after a write may hit stale cache until invalidation completes
- Under write-heavy workloads, changelog reads can bottleneck

**Impact:** Medium - Affects consistency and can cause cache misses in bursts.

---

### 6. **No Cross-Batch Check Deduplication** (Medium Impact)

**Location:** `batch_check_command.go:150-166`

**Problem:** Deduplication only occurs within a single batch request. Concurrent batch requests checking the same permissions don't share results.

**Current Mitigation:** The `CachedCheckResolver` provides some cross-request caching, but:
- Only caches final resolved results, not in-flight computations
- Doesn't use singleflight for concurrent identical checks

**Impact:** Medium - Wasted computation when multiple concurrent batches check identical permissions.

---

### 7. **Unbounded Contextual Tuples Serialization** (Medium Impact)

**Location:** `batch_check_command.go:282-304`

```go
func generateCacheKeyFromCheck(check *openfgav1.BatchCheckItem, ...) (CacheKey, error) {
    cacheKeyParams := &storage.CheckCacheKeyParams{
        // ...
        ContextualTuples: check.GetContextualTuples().GetTupleKeys(),
        Context:          check.GetContext(),
    }
    hasher := xxhash.New()
    err := storage.WriteCheckCacheKey(hasher, cacheKeyParams)
    // ...
}
```

**Problem:** Checks with large contextual tuple sets or complex context payloads incur significant serialization overhead for cache key generation.

**Impact:** Medium - Depends on usage patterns; can be significant for requests with many contextual tuples.

---

### 8. **Response Cloning Overhead** (Low Impact)

**Location:** `cached_resolver.go:176, 208`

```go
// Return a copy to avoid races across goroutines
return res.CheckResponse.clone(), nil
// ...
clonedResp := resp.clone()
c.cache.Set(cacheKey, &CheckResponseCacheEntry{...CheckResponse: clonedResp}, c.cacheTTL)
```

**Problem:** Every cache hit and cache write involves cloning the response. While necessary for thread safety, this adds allocation overhead.

**Impact:** Low - Trade-off for correctness; unavoidable without restructuring.

---

### 9. **Datastore Throttling Adds Latency** (Configurable Impact)

**Location:** `bounded_datastore.go:209-218`

```go
if b.throttlingEnabled && b.threshold > 0 && reads > b.threshold {
    b.throttled.Store(true)
    select {
    case <-ctx.Done():
        return ctx.Err()
    case <-time.After(b.throttleTime):  // Default: 10µs
        break
    }
}
```

**Problem:** When datastore throttling is enabled and threshold is exceeded, artificial latency is added. This is by design to protect the database, but can compound in batch checks.

**Impact:** Varies - Depends on threshold configuration and check complexity.

---

### 10. **Single Check Resolver Per Request** (Architectural)

**Location:** `batch_check.go:56-61`

```go
builder := s.getCheckResolverBuilder(req.GetStoreId())
checkResolver, checkResolverCloser, err := builder.Build()
// ...
defer checkResolverCloser()
```

**Problem:** A new check resolver chain is built per BatchCheck request. While the cache is shared, resolver construction has overhead.

**Impact:** Low - Resolver construction is lightweight, but could be pooled.

---

## Configuration Bottlenecks

### Default Settings Analysis

| Setting | Default | Potential Issue |
|---------|---------|-----------------|
| `maxChecksPerBatchCheck` | 50 | May be too low for some use cases |
| `maxConcurrentChecksPerBatchCheck` | 50 | Equal to max checks - no queuing benefit |
| `checkCacheLimit` | 10,000 | May be insufficient for large deployments |
| `checkQueryCacheTTL` | 10s | Short TTL increases cache misses |
| `cacheControllerTTL` | 10s | Frequent invalidation checks |

---

## Recommendations

### High Priority

#### 1. Implement Cross-Request Singleflight

Add singleflight for identical in-flight checks across concurrent requests:

```go
// In CachedCheckResolver.ResolveCheck()
result, err, _ := c.singleflight.Do(cacheKey, func() (interface{}, error) {
    return c.delegate.ResolveCheck(ctx, req)
})
```

**Benefit:** Eliminates redundant computation for concurrent identical checks.

#### 2. Consider Streaming Results

For large batches, implement gRPC streaming to return results as they complete:

```go
// Stream results as checks complete
for outcome := range resultChan {
    stream.Send(&BatchCheckPartialResult{...})
}
```

**Benefit:** Clients can process results incrementally; faster perceived latency.

#### 3. Pool CheckQuery Objects

Implement object pooling for `CheckQuery`:

```go
var checkQueryPool = sync.Pool{
    New: func() interface{} {
        return &CheckQuery{}
    },
}
```

**Benefit:** Reduces GC pressure for high-throughput scenarios.

### Medium Priority

#### 4. Replace sync.Map with Pre-sized Map

```go
results := make(map[CacheKey]*BatchCheckOutcome, len(cacheKeyMap))
var mu sync.Mutex

// In goroutine:
mu.Lock()
results[key] = outcome
mu.Unlock()
```

**Benefit:** Better performance for write-once-read-once pattern.

#### 5. Lazy Cache Key Computation

Defer cache key computation until deduplication is actually needed:

```go
// Only compute hash if we've seen >1 check
if len(checks) > 1 {
    // Compute cache keys
}
```

**Benefit:** Avoids overhead for small batches.

#### 6. Batch Changelog Reads

In `CacheController`, batch changelog reads across stores:

```go
// Instead of per-store invalidation
changesMap := c.ds.ReadChangesMultiStore(ctx, storeIDs, ...)
```

**Benefit:** Reduces database round trips for multi-store deployments.

### Low Priority

#### 7. Configurable Deduplication

Add option to disable deduplication for batches with mostly unique checks:

```go
if bq.deduplicationEnabled && len(params.Checks) > 1 {
    // Perform deduplication
}
```

#### 8. Contextual Tuple Caching

For checks with large contextual tuple sets, consider:
- Pre-computing contextual tuple hashes
- Caching contextual tuple sets separately

#### 9. Metrics for Optimization Guidance

Add metrics to identify optimization opportunities:
- `batch_check_deduplication_ratio` - Unique checks / total checks
- `batch_check_slowest_check_duration` - Identify slow checks
- `batch_check_cache_hit_ratio` - Per-batch cache effectiveness

---

## Performance Testing Recommendations

1. **Baseline Measurements**
   - Measure p50/p95/p99 latency for batch sizes: 10, 25, 50
   - Profile memory allocation patterns
   - Measure cache hit rates

2. **Load Testing Scenarios**
   - Concurrent identical batches (test singleflight benefit)
   - Batches with mixed cache hit/miss ratios
   - Write-heavy workloads (cache invalidation stress)

3. **Profiling Focus Areas**
   - `generateCacheKeyFromCheck` - hash computation overhead
   - `pool.Wait()` - blocking time distribution
   - GC pause times under load

---

## Summary Table

| Bottleneck | Severity | Effort to Fix | ROI |
|------------|----------|---------------|-----|
| Synchronous Pool.Wait() | High | High | Medium |
| No cross-request deduplication | Medium | Medium | High |
| Per-check object allocation | Medium | Low | Medium |
| Cache invalidation latency | Medium | Medium | Medium |
| sync.Map overhead | Low | Low | Low |
| Contextual tuple serialization | Medium | Medium | Medium |
| Response cloning | Low | N/A | N/A |

---

## Conclusion

The BatchCheck implementation is well-designed with good use of:
- Parallel execution via goroutine pools
- Request-level deduplication
- Multi-layer caching (check response + iterator)
- Configurable throttling

Key areas for improvement:
1. **Cross-request coordination** - Singleflight for concurrent identical checks
2. **Result streaming** - Don't wait for the slowest check
3. **Object pooling** - Reduce GC pressure at scale

The most impactful quick win would be implementing singleflight in the `CachedCheckResolver` to eliminate redundant computation for concurrent identical checks across batch requests.
