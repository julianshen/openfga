# OpenFGA Architecture Review: Scalability, Performance & Reimplementation Analysis

## Executive Summary

This document provides a comprehensive architecture review of OpenFGA, identifying bottlenecks, scalability challenges, and providing recommendations for refactoring, re-architecture, and potential Rust reimplementation.

**Key Findings:**
- OpenFGA has a well-designed layered architecture with pluggable storage backends
- Primary bottlenecks are in graph traversal algorithms, cache invalidation, and batch operation coordination
- Horizontal scaling is limited by single-node graph resolution and synchronous cache invalidation
- A Rust reimplementation could yield 2-5x performance improvements with proper design
- Storage backend choice significantly impacts performance characteristics at scale

---

## Table of Contents

1. [Current Architecture Overview](#1-current-architecture-overview)
2. [Identified Bottlenecks](#2-identified-bottlenecks)
3. [Scalability Challenges for Large Deployments](#3-scalability-challenges-for-large-deployments)
4. [Refactoring Recommendations](#4-refactoring-recommendations)
5. [Rust Reimplementation Suggestions](#5-rust-reimplementation-suggestions)
6. [Storage Backend Considerations](#6-storage-backend-considerations)
7. [Data Schema Recommendations](#7-data-schema-recommendations)
8. [Implementation Roadmap](#8-implementation-roadmap)

---

## 1. Current Architecture Overview

### 1.1 High-Level Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    API Layer (gRPC + HTTP)                  │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ Middleware: Auth, Logging, Validation, Metrics, Trace│   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                    Server Layer                             │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐        │
│  │ Check API    │ │ BatchCheck   │ │ ListObjects  │        │
│  │ Handler      │ │ Handler      │ │ Handler      │        │
│  └──────────────┘ └──────────────┘ └──────────────┘        │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐        │
│  │ Write API    │ │ Read API     │ │ Store CRUD   │        │
│  └──────────────┘ └──────────────┘ └──────────────┘        │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                    Domain Layer                             │
│  ┌──────────────────┐  ┌──────────────────┐                │
│  │ Graph Resolution │  │ Type System      │                │
│  │ (Check Algorithm)│  │ (Model Parsing)  │                │
│  └──────────────────┘  └──────────────────┘                │
│  ┌──────────────────┐  ┌──────────────────┐                │
│  │ Planner          │  │ Condition (CEL)  │                │
│  │ (Query Optimizer)│  │ Evaluator        │                │
│  └──────────────────┘  └──────────────────┘                │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                Storage Wrapper Layer                        │
│  ┌────────────┐ ┌────────────┐ ┌────────────┐ ┌──────────┐ │
│  │ Cached     │ │ Bounded    │ │ Shared     │ │ Combined │ │
│  │ Datastore  │ │ Reader     │ │ Iterator   │ │ Reader   │ │
│  └────────────┘ └────────────┘ └────────────┘ └──────────┘ │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                 Storage Backend Layer                       │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌────────┐ ┌──────┐│
│  │PostgreSQL│ │ MySQL    │ │ SQLite   │ │ Memory │ │Valkey││
│  └──────────┘ └──────────┘ └──────────┘ └────────┘ └──────┘│
└─────────────────────────────────────────────────────────────┘
```

### 1.2 Key Design Patterns

| Pattern | Location | Purpose |
|---------|----------|---------|
| Functional Options | `pkg/server/server.go` | Flexible configuration |
| Storage Interface | `pkg/storage/storage.go` | Backend abstraction |
| Decorator/Wrapper | `pkg/storage/storagewrappers/` | Layered caching/throttling |
| Command Pattern | `pkg/server/commands/` | Business logic encapsulation |
| Resolver Chain | `internal/graph/` | Composable graph resolution |
| Singleflight | Throughout | Request deduplication |

### 1.3 Request Flow (Check API)

```
1. gRPC/HTTP Request
2. Middleware Chain (Auth → Logging → Validation → Metrics)
3. Server.Check() Handler
   ├─ Build CheckResolver chain
   ├─ Resolve TypeSystem (cached)
   └─ Execute CheckCommand
        ├─ Build Storage Wrapper stack
        ├─ ResolveCheck() → Graph traversal
        │   ├─ Cached lookup (if enabled)
        │   ├─ Direct tuple match
        │   ├─ Userset evaluation
        │   └─ Recursive resolution (TTU)
        └─ Return CheckResponse
4. Response with metrics
```

---

## 2. Identified Bottlenecks

### 2.1 Critical Bottlenecks (High Impact)

#### 2.1.1 Synchronous Batch Wait

**Location:** `pkg/server/commands/batch_check_command.go:234`

```go
_ = pool.Wait()  // Blocks until ALL checks complete
```

**Problem:** Batch latency = max(individual check latencies). A single slow check delays the entire response.

**Impact:**
- p99 latency spikes with heterogeneous check complexity
- No partial result streaming
- Wasted client time waiting for slowest check

**Severity:** HIGH

---

#### 2.1.2 No Cross-Request Singleflight for Check Resolution

**Location:** `internal/graph/cached_resolver.go`

**Problem:** Multiple concurrent requests checking identical permissions compute independently. Only the cache layer provides deduplication, but cache misses cause redundant computation.

**Impact:**
- Wasted CPU during traffic spikes
- Redundant database queries
- Higher p99 latency under concurrent identical requests

**Severity:** HIGH

---

#### 2.1.3 Cache Invalidation Latency

**Location:** `internal/cachecontroller/cache_controller.go:207-260`

```go
go func() {
    c.findChangesAndInvalidateIfNecessary(ctx, storeID)  // Async with 1s timeout
}()
```

**Problem:**
- Async invalidation with 1-second timeout
- Only one invalidation per store at a time
- First check after write may hit stale cache
- High write volume causes changelog read bottleneck

**Impact:**
- Stale authorization decisions after writes
- Inconsistency window of up to 1 second
- Potential security implications

**Severity:** HIGH

---

### 2.2 Medium Impact Bottlenecks

#### 2.2.1 Per-Check Object Allocation

**Location:** `pkg/server/commands/batch_check_command.go:188-199`

**Problem:** New `CheckQuery` struct allocated per check in batch with multiple options applied.

**Impact:** GC pressure at high throughput (thousands of RPS)

**Severity:** MEDIUM

---

#### 2.2.2 Graph Traversal Depth Limits

**Location:** `pkg/server/config/config.go:22-23`

```go
DefaultResolveNodeLimit        = 25
DefaultResolveNodeBreadthLimit = 10
```

**Problem:** Deep authorization models hit resolution limits, causing query failures.

**Impact:** Complex hierarchical models may not be fully resolvable

**Severity:** MEDIUM

---

#### 2.2.3 sync.Map Overhead in Result Collection

**Location:** `pkg/server/commands/batch_check_command.go:168`

**Problem:** `sync.Map` optimized for read-heavy workloads, but batch check is write-once-read-once.

**Impact:** Suboptimal performance for batch result collection

**Severity:** LOW-MEDIUM

---

#### 2.2.4 Contextual Tuple Serialization

**Location:** `pkg/server/commands/batch_check_command.go:282-304`

**Problem:** Large contextual tuple sets require expensive serialization for cache key generation.

**Impact:** High overhead for "what-if" authorization checks

**Severity:** MEDIUM

---

### 2.3 Low Impact Bottlenecks

| Bottleneck | Location | Impact |
|------------|----------|--------|
| Response cloning | `cached_resolver.go:176,208` | Minor allocation overhead |
| Hash computation per check | `batch_check_command.go:150` | CPU overhead for deduplication |
| Resolver chain construction | `batch_check.go:56-61` | Per-request overhead |

---

## 3. Scalability Challenges for Large Deployments

### 3.1 Horizontal Scaling Limitations

#### 3.1.1 Single-Node Graph Resolution

**Problem:** Graph traversal occurs entirely within a single node. No distributed graph resolution.

**Implications:**
- Cannot parallelize complex checks across multiple nodes
- Single node becomes bottleneck for deep graph traversal
- Vertical scaling required for complex authorization models

**Potential Solutions:**
1. Distributed graph computation (like Pregel/BSP model)
2. Graph partitioning with cross-partition coordination
3. Materialized views for common access patterns

---

#### 3.1.2 Per-Node Cache State

**Problem:** Each OpenFGA instance maintains independent caches (check cache, iterator cache, model cache).

**Implications:**
- Cache hit rate decreases with more instances
- Inconsistent authorization decisions during cache warm-up
- Inefficient memory utilization across cluster

**Potential Solutions:**
1. Distributed cache (Redis/Valkey for check results)
2. Cache replication protocol
3. Consistent hashing for request routing

---

#### 3.1.3 Single Database Bottleneck

**Problem:** All instances share the same database, creating contention.

**Implications for 1M+ tuples:**
- Write conflicts during high write volume
- Index maintenance overhead
- Query planning variability

**Potential Solutions:**
1. Read replicas with eventual consistency
2. Sharding by store_id or object_type
3. Write-behind caching for batched persistence

---

### 3.2 Large-Scale Deployment Challenges

#### 3.2.1 Multi-Tenant Isolation

**Current State:** Stores provide logical isolation, but share:
- Database connection pools
- Cache space
- CPU resources

**Challenges:**
- Noisy neighbor problems
- Fair resource allocation
- Per-tenant throttling granularity

---

#### 3.2.2 Model Complexity at Scale

| Metric | Limit | Concern at Scale |
|--------|-------|------------------|
| Types per model | 100 | May be insufficient for complex domains |
| Resolve depth | 25 | Deep hierarchies fail |
| Breadth limit | 10 | Wide permission trees truncated |
| Tuples per write | 100 | Bulk import limited |

---

#### 3.2.3 Changelog Growth

**Problem:** Changelog table grows unbounded without explicit purging.

**Implications:**
- Storage cost increases linearly
- Cache invalidation queries slow down
- Migration/backup times increase

---

### 3.3 Estimated Scale Limits (Current Architecture)

| Metric | Estimated Limit | Bottleneck |
|--------|----------------|------------|
| Tuples per store | ~100M | Database query performance |
| Checks per second (single node) | ~10K | CPU-bound graph resolution |
| Checks per second (cluster) | ~50K | Database connection pool |
| Batch checks per second | ~500 | Synchronous wait pattern |
| Stores per deployment | ~10K | Cache efficiency degradation |
| Concurrent connections | ~1000 | gRPC server limits |

---

## 4. Refactoring Recommendations

### 4.1 High Priority Refactoring

#### 4.1.1 Implement Cross-Request Singleflight

**Current:** Only iterator cache uses singleflight
**Proposed:** Add singleflight to CachedCheckResolver

```go
// internal/graph/cached_resolver.go
type CachedCheckResolver struct {
    delegate     CheckResolver
    cache        storage.InMemoryCache[any]
    singleflight singleflight.Group  // ADD THIS
    cacheTTL     time.Duration
}

func (c *CachedCheckResolver) ResolveCheck(ctx context.Context, req *ResolveCheckRequest) (*ResolveCheckResponse, error) {
    cacheKey := buildCacheKey(req)

    // Check cache first
    if cached, found := c.cache.Get(cacheKey); found {
        return cached.clone(), nil
    }

    // Use singleflight for in-flight deduplication
    result, err, _ := c.singleflight.Do(cacheKey, func() (interface{}, error) {
        return c.delegate.ResolveCheck(ctx, req)
    })

    if err != nil {
        return nil, err
    }

    resp := result.(*ResolveCheckResponse)
    c.cache.Set(cacheKey, resp.clone(), c.cacheTTL)
    return resp, nil
}
```

**Impact:** Eliminates redundant computation for concurrent identical checks

---

#### 4.1.2 Streaming Batch Results

**Current:** Synchronous wait for all checks
**Proposed:** gRPC streaming for batch results

```protobuf
// New streaming RPC
rpc StreamBatchCheck(BatchCheckRequest) returns (stream BatchCheckPartialResult);

message BatchCheckPartialResult {
    string correlation_id = 1;
    bool allowed = 2;
    oneof error {
        string error_message = 3;
    }
}
```

**Benefits:**
- Clients process results as available
- Faster perceived latency
- Better resource utilization

---

#### 4.1.3 Synchronous Cache Invalidation for Writes

**Current:** Async invalidation with potential stale reads
**Proposed:** Synchronous in-memory timestamp update

```go
func (c *InMemoryCacheController) InvalidateOnWrite(storeID string, writeTime time.Time) {
    c.lastWriteTime.Store(storeID, writeTime)  // Immediate

    go func() {
        // Async propagation to distributed cache
        c.propagateInvalidation(storeID, writeTime)
    }()
}
```

---

### 4.2 Medium Priority Refactoring

#### 4.2.1 Object Pooling for Check Commands

```go
var checkQueryPool = sync.Pool{
    New: func() interface{} {
        return &CheckQuery{}
    },
}

func (bq *BatchCheckQuery) Execute(ctx context.Context, params *BatchCheckCommandParams) (*BatchCheckOutcome, error) {
    pool.Go(func(ctx context.Context) error {
        checkQuery := checkQueryPool.Get().(*CheckQuery)
        defer checkQueryPool.Put(checkQuery)

        checkQuery.Reset()  // Clear state
        checkQuery.Configure(bq.datastore, bq.typesys, ...)

        // Execute...
    })
}
```

---

#### 4.2.2 Replace sync.Map with Pre-sized Map

```go
// Before
var resultMap = new(sync.Map)

// After
results := make(map[CacheKey]*BatchCheckOutcome, len(cacheKeyMap))
var mu sync.Mutex

pool.Go(func(ctx context.Context) error {
    // ... execute check ...
    mu.Lock()
    results[key] = outcome
    mu.Unlock()
    return nil
})
```

---

#### 4.2.3 Lazy Cache Key Computation

```go
func (bq *BatchCheckQuery) Execute(ctx context.Context, params *BatchCheckCommandParams) {
    checks := params.Checks

    // Skip deduplication for small batches
    if len(checks) <= 3 {
        return bq.executeWithoutDeduplication(ctx, checks)
    }

    // Compute cache keys only when beneficial
    return bq.executeWithDeduplication(ctx, checks)
}
```

---

### 4.3 Architecture Improvements

#### 4.3.1 Introduce Result Cache Tier

```
┌─────────────────────────────────────────────────────────────┐
│                    L1: In-Process Cache                     │
│  (LRU, TTL-based, per-instance)                            │
└─────────────────────────────────────────────────────────────┘
                              │ (miss)
┌─────────────────────────────────────────────────────────────┐
│                    L2: Distributed Cache                    │
│  (Redis/Valkey, shared across instances)                   │
└─────────────────────────────────────────────────────────────┘
                              │ (miss)
┌─────────────────────────────────────────────────────────────┐
│                    L3: Graph Computation                    │
│  (Resolve from database)                                   │
└─────────────────────────────────────────────────────────────┘
```

---

#### 4.3.2 Materialized Permission Views

For frequently-checked permissions, pre-compute and store results:

```sql
CREATE TABLE materialized_permissions (
    store_id VARCHAR(26),
    user_type VARCHAR(256),
    user_id VARCHAR(256),
    relation VARCHAR(50),
    object_type VARCHAR(256),
    object_id VARCHAR(256),
    allowed BOOLEAN,
    computed_at TIMESTAMP,
    expires_at TIMESTAMP,
    PRIMARY KEY (store_id, user_type, user_id, relation, object_type, object_id)
);
```

Trigger updates on tuple writes, use for common access patterns.

---

#### 4.3.3 Query Planner Improvements

Current planner selects between resolution strategies. Enhance with:

1. **Cost-based optimization**: Estimate tuple counts before execution
2. **Index hints**: Prefer queries with matching indexes
3. **Cardinality estimation**: Better handling of high-cardinality relations
4. **Query plan caching**: Reuse plans for identical query shapes

---

## 5. Rust Reimplementation Suggestions

### 5.1 Why Rust?

| Aspect | Go (Current) | Rust (Proposed) |
|--------|--------------|-----------------|
| Memory management | GC pauses (~1-10ms) | Zero-cost abstractions |
| Concurrency | Goroutines (cheap) | Async/await + Tokio |
| Performance | Good | Excellent (2-5x potential) |
| Memory safety | GC-managed | Compile-time guarantees |
| Binary size | ~50MB | ~10MB (static) |
| Startup time | ~100ms | ~10ms |

### 5.2 Recommended Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    API Layer (Tonic gRPC)                   │
│  Axum for HTTP, Tower middleware                           │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                    Service Layer                            │
│  Async trait-based handlers                                │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                    Domain Layer                             │
│  ┌──────────────────┐  ┌──────────────────┐                │
│  │ GraphResolver    │  │ TypeSystem       │                │
│  │ (petgraph-based) │  │ (serde models)   │                │
│  └──────────────────┘  └──────────────────┘                │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                    Storage Layer                            │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐        │
│  │ SQLx         │ │ Redis-rs     │ │ Custom B-Tree│        │
│  │ (Postgres)   │ │ (Valkey)     │ │ (Embedded)   │        │
│  └──────────────┘ └──────────────┘ └──────────────┘        │
└─────────────────────────────────────────────────────────────┘
```

### 5.3 Key Crates Recommendations

| Component | Recommended Crate | Purpose |
|-----------|------------------|---------|
| gRPC | `tonic` + `prost` | High-performance gRPC |
| HTTP | `axum` + `tower` | HTTP/REST gateway |
| Async runtime | `tokio` | Async I/O |
| Database | `sqlx` | Compile-time checked SQL |
| Redis | `redis-rs` | Valkey/Redis client |
| Caching | `moka` | Concurrent cache |
| Graph | `petgraph` | Graph data structures |
| CEL | Custom or `cel-rust` | Condition evaluation |
| Serialization | `serde` + `prost` | JSON/Protobuf |
| Metrics | `prometheus` | Metrics export |
| Tracing | `tracing` + `opentelemetry` | Distributed tracing |

### 5.4 Core Data Structures

```rust
// Tuple representation with arena allocation
#[derive(Clone, Debug)]
pub struct Tuple {
    pub store_id: StoreId,
    pub object: TypedObject,
    pub relation: RelationName,
    pub user: Subject,
    pub condition: Option<Condition>,
    pub ulid: Ulid,
}

// Interned strings for memory efficiency
#[derive(Clone, Copy, Hash, Eq, PartialEq)]
pub struct TypeName(u32);  // Index into string interner

#[derive(Clone, Copy, Hash, Eq, PartialEq)]
pub struct RelationName(u32);

// Graph structure for permission resolution
pub struct PermissionGraph {
    nodes: Vec<Node>,
    edges: Vec<Edge>,
    type_index: HashMap<TypeName, Vec<NodeId>>,
    relation_index: HashMap<(TypeName, RelationName), Vec<EdgeId>>,
}

// Check resolver with compile-time dispatch
pub trait CheckResolver: Send + Sync {
    async fn resolve(&self, req: &CheckRequest) -> Result<CheckResponse, Error>;
}

// Composable resolver chain
pub struct ResolverChain {
    cache: CachedResolver,
    throttle: ThrottledResolver,
    core: CoreResolver,
}
```

### 5.5 Performance Optimizations

#### 5.5.1 Zero-Copy Parsing

```rust
// Use bytes::Bytes for zero-copy buffer handling
pub struct TupleIterator<'a> {
    buffer: &'a [u8],
    cursor: usize,
}

impl<'a> Iterator for TupleIterator<'a> {
    type Item = TupleRef<'a>;  // Borrowed reference, no allocation

    fn next(&mut self) -> Option<Self::Item> {
        // Parse directly from buffer without copying
    }
}
```

#### 5.5.2 Lock-Free Caching

```rust
use moka::future::Cache;

pub struct CheckCache {
    cache: Cache<CheckCacheKey, Arc<CheckResponse>>,
}

impl CheckCache {
    pub async fn get_or_compute<F, Fut>(
        &self,
        key: CheckCacheKey,
        compute: F,
    ) -> Result<Arc<CheckResponse>, Error>
    where
        F: FnOnce() -> Fut,
        Fut: Future<Output = Result<CheckResponse, Error>>,
    {
        self.cache
            .try_get_with(key, async { compute().await.map(Arc::new) })
            .await
    }
}
```

#### 5.5.3 SIMD-Accelerated Comparisons

```rust
// For bulk tuple matching
#[cfg(target_arch = "x86_64")]
use std::arch::x86_64::*;

pub fn match_tuples_simd(
    tuples: &[Tuple],
    filter: &TupleFilter,
) -> Vec<&Tuple> {
    // Use SIMD for parallel comparison of object IDs
    // 4-8x speedup for large tuple sets
}
```

### 5.6 API Compatibility

Maintain full API compatibility with existing OpenFGA:

```rust
// Proto definitions remain identical
pub mod openfga_v1 {
    tonic::include_proto!("openfga.v1");
}

// Service implementation
#[tonic::async_trait]
impl OpenFgaService for OpenFgaServer {
    async fn check(
        &self,
        request: Request<CheckRequest>,
    ) -> Result<Response<CheckResponse>, Status> {
        // Compatible implementation
    }

    async fn batch_check(
        &self,
        request: Request<BatchCheckRequest>,
    ) -> Result<Response<BatchCheckResponse>, Status> {
        // Stream results internally, return complete response
    }
}
```

### 5.7 Migration Strategy

1. **Phase 1: Core Library**
   - Implement tuple storage and graph resolution in Rust
   - Expose as C FFI for gradual integration
   - Benchmark against Go implementation

2. **Phase 2: Standalone Service**
   - Full gRPC service in Rust
   - Same proto definitions
   - Drop-in replacement for Go version

3. **Phase 3: Enhanced Features**
   - Distributed graph resolution
   - Advanced caching strategies
   - Custom storage backends

---

## 6. Storage Backend Considerations

### 6.1 Current Backend Comparison

| Backend | Strengths | Weaknesses | Best For |
|---------|-----------|------------|----------|
| PostgreSQL | ACID, mature, indexes | Connection overhead | Production, complex queries |
| MySQL | Wide adoption | Lock contention | Existing MySQL infrastructure |
| SQLite | Embedded, simple | Single-writer lock | Development, testing |
| Memory | Fast, simple | No persistence | Testing only |
| Valkey | Distributed, fast | No ACID, consistency | High-read, caching tier |

### 6.2 Recommended Production Setup

```
┌─────────────────────────────────────────────────────────────┐
│                    OpenFGA Cluster                          │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐                    │
│  │ Instance │ │ Instance │ │ Instance │                    │
│  │    1     │ │    2     │ │    3     │                    │
│  └────┬─────┘ └────┬─────┘ └────┬─────┘                    │
│       └────────────┼────────────┘                          │
└───────────────────────────────────────────────────────────┘
                     │
        ┌────────────┴────────────┐
        │                         │
┌───────▼───────┐        ┌───────▼───────┐
│  Valkey       │        │  PostgreSQL   │
│  (L2 Cache)   │        │  (Primary)    │
│  Cluster      │        │  + Read       │
│               │        │    Replicas   │
└───────────────┘        └───────────────┘
```

### 6.3 New Backend Recommendations

#### 6.3.1 TiKV/TiDB

**Advantages:**
- Distributed, horizontally scalable
- Strong consistency (Raft)
- Automatic sharding
- Compatible with MySQL protocol (TiDB)

**Use Case:** Multi-region deployments, 1B+ tuples

---

#### 6.3.2 CockroachDB

**Advantages:**
- PostgreSQL-compatible
- Distributed SQL
- Automatic rebalancing
- Serializable isolation

**Use Case:** Global deployments, strong consistency required

---

#### 6.3.3 FoundationDB

**Advantages:**
- Excellent for custom data models
- ACID transactions
- Layer abstraction
- Proven at Apple scale

**Use Case:** Custom optimization, extreme scale

---

#### 6.3.4 ScyllaDB

**Advantages:**
- C++ rewrite of Cassandra
- Extremely fast
- Predictable latency
- Linear scalability

**Use Case:** Read-heavy workloads, eventual consistency acceptable

---

### 6.4 Embedded Engine for Edge Deployment

For edge/embedded scenarios, consider:

```rust
// Custom B-Tree based storage
pub struct EmbeddedStorage {
    tuples: BTreeMap<TupleKey, TupleValue>,
    indexes: SecondaryIndexes,
    wal: WriteAheadLog,
}

// Or use existing embedded databases
// - sled (pure Rust)
// - RocksDB (via rust-rocksdb)
// - LMDB (via lmdb-rs)
```

---

## 7. Data Schema Recommendations

### 7.1 Current Schema Analysis

**Strengths:**
- Simple, normalized tuple storage
- Efficient primary key lookups
- Changelog for audit/cache invalidation

**Weaknesses:**
- No pre-computed transitive closure
- Expensive reverse lookups
- Index explosion for complex queries

### 7.2 Optimized Schema (Same Semantics)

```sql
-- Core tuple table with better indexing
CREATE TABLE tuples (
    id BIGINT GENERATED ALWAYS AS IDENTITY,
    store_id CHAR(26) NOT NULL,

    -- Object columns
    object_type VARCHAR(256) NOT NULL,
    object_id VARCHAR(256) NOT NULL,

    -- Relation
    relation VARCHAR(50) NOT NULL,

    -- User columns (denormalized for query efficiency)
    user_type VARCHAR(256) NOT NULL,
    user_id VARCHAR(256) NOT NULL,
    user_relation VARCHAR(50),  -- For userset subjects

    -- Condition (optional)
    condition_name VARCHAR(256),
    condition_context JSONB,

    -- Metadata
    ulid CHAR(26) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),

    -- Composite primary key for uniqueness
    CONSTRAINT pk_tuples PRIMARY KEY (
        store_id, object_type, object_id, relation,
        user_type, user_id, COALESCE(user_relation, '')
    )
);

-- Optimized indexes
CREATE INDEX idx_tuples_object_relation
    ON tuples (store_id, object_type, object_id, relation)
    INCLUDE (user_type, user_id, user_relation, condition_name);

CREATE INDEX idx_tuples_user_reverse
    ON tuples (store_id, user_type, user_id, user_relation, object_type, relation)
    INCLUDE (object_id);

CREATE INDEX idx_tuples_type_relation
    ON tuples (store_id, object_type, relation)
    WHERE user_type = 'user';  -- Partial index for direct users

CREATE INDEX idx_tuples_ulid
    ON tuples (store_id, ulid DESC);
```

### 7.3 New Schema (Optimized for Performance)

```sql
-- Graph-optimized schema

-- Node table (objects and users)
CREATE TABLE nodes (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    store_id CHAR(26) NOT NULL,
    node_type VARCHAR(256) NOT NULL,
    node_id VARCHAR(256) NOT NULL,

    UNIQUE (store_id, node_type, node_id)
);

-- Edge table (relationships)
CREATE TABLE edges (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    store_id CHAR(26) NOT NULL,

    -- Source (object)
    source_node_id BIGINT NOT NULL REFERENCES nodes(id),

    -- Relation
    relation_id SMALLINT NOT NULL,  -- FK to relations table

    -- Target (user)
    target_node_id BIGINT NOT NULL REFERENCES nodes(id),
    target_relation_id SMALLINT,  -- For userset subjects

    -- Condition
    condition_id INT REFERENCES conditions(id),

    -- Metadata
    ulid CHAR(26) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),

    UNIQUE (store_id, source_node_id, relation_id, target_node_id,
            COALESCE(target_relation_id, 0))
);

-- Relation dictionary (for compression)
CREATE TABLE relations (
    id SMALLINT PRIMARY KEY,
    store_id CHAR(26) NOT NULL,
    name VARCHAR(50) NOT NULL,
    UNIQUE (store_id, name)
);

-- Pre-computed transitive closure (optional, for read-heavy workloads)
CREATE TABLE transitive_closure (
    store_id CHAR(26) NOT NULL,
    source_node_id BIGINT NOT NULL,
    relation_id SMALLINT NOT NULL,
    target_node_id BIGINT NOT NULL,
    path_length SMALLINT NOT NULL,
    computed_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,

    PRIMARY KEY (store_id, source_node_id, relation_id, target_node_id)
);

-- Efficient indexes
CREATE INDEX idx_edges_outgoing
    ON edges (store_id, source_node_id, relation_id);

CREATE INDEX idx_edges_incoming
    ON edges (store_id, target_node_id, target_relation_id, relation_id);

CREATE INDEX idx_tc_lookup
    ON transitive_closure (store_id, target_node_id, relation_id, source_node_id)
    WHERE expires_at > NOW();
```

### 7.4 Schema Migration Strategy

1. **Parallel Write**: Write to both old and new schema during transition
2. **Shadow Read**: Compare results from both schemas
3. **Gradual Cutover**: Route increasing traffic to new schema
4. **Cleanup**: Remove old schema after validation

---

## 8. Implementation Roadmap

### 8.1 Short-Term (1-3 months)

| Priority | Task | Impact | Effort |
|----------|------|--------|--------|
| P0 | Implement cross-request singleflight | High | Low |
| P0 | Synchronous cache invalidation on write | High | Medium |
| P1 | Object pooling for CheckQuery | Medium | Low |
| P1 | Replace sync.Map in batch check | Low | Low |
| P2 | Add batch check metrics (deduplication ratio, slowest check) | Low | Low |

### 8.2 Medium-Term (3-6 months)

| Priority | Task | Impact | Effort |
|----------|------|--------|--------|
| P0 | Streaming batch check results (gRPC) | High | Medium |
| P1 | L2 distributed cache (Valkey) | High | Medium |
| P1 | Query planner improvements | Medium | Medium |
| P2 | Materialized permission views (experimental) | Medium | High |
| P2 | Schema optimization (indexes, partitioning) | Medium | Medium |

### 8.3 Long-Term (6-12 months)

| Priority | Task | Impact | Effort |
|----------|------|--------|--------|
| P0 | Rust core library (graph resolution) | High | High |
| P1 | Distributed graph computation | High | Very High |
| P1 | New storage backends (TiKV, CockroachDB) | Medium | High |
| P2 | Full Rust reimplementation | High | Very High |
| P2 | Edge deployment support (embedded engine) | Medium | High |

---

## 9. Conclusion

OpenFGA is a well-architected authorization system with solid foundations. The identified bottlenecks are addressable through targeted refactoring:

1. **Quick Wins**: Singleflight, object pooling, sync.Map replacement
2. **Medium Effort**: Streaming results, distributed caching
3. **Strategic Investment**: Rust reimplementation, distributed graph resolution

For very large deployments (100M+ tuples, 100K+ RPS), consider:
- Distributed graph computation
- Purpose-built storage backend (TiKV/CockroachDB)
- Materialized permission views
- Multi-tier caching architecture

A Rust reimplementation could provide 2-5x performance improvement while maintaining full API compatibility, making it an attractive option for organizations with extreme performance requirements.

---

*Document Version: 1.0*
*Last Updated: 2025-12-31*
*Author: Architecture Review*
