# rsfga: Technical Specification

**Version**: 1.0
**Status**: Draft
**Last Updated**: 2025-12-31

---

## 1. Overview

This specification defines the technical details for rsfga (Rust FGA) implementation, including data structures, APIs, protocols, and algorithms.

---

## 2. Data Structures

### 2.1 Edge Storage Schema

#### 2.1.1 PrecomputedCheck

The core data structure storing pre-computed authorization results.

```rust
/// Key: xxhash64(store_id, object, relation, user)
/// Size: 8 bytes (key) + variable (value)

pub struct PrecomputedCheck {
    /// True if access is allowed without any conditions
    /// Size: 1 byte
    allowed_unconditional: bool,

    /// Paths that may allow access if conditions pass
    /// Size: variable (typically 0-3 paths)
    conditional_paths: Vec<ConditionalPath>,

    /// Timestamp when this entry was last updated
    /// Size: 8 bytes
    updated_at: u64,
}
```

#### 2.1.2 ConditionalPath

Represents a single path through the authorization graph that requires condition evaluation.

```rust
pub struct ConditionalPath {
    /// Reference to the condition definition
    /// Size: ~32 bytes (string)
    condition_name: String,

    /// Parameters bound at tuple creation time
    /// Size: variable (typically 1-5 params)
    bound_params: HashMap<String, ConditionValue>,

    /// Parameters required from the Check request context
    /// Size: variable (typically 1-3 params)
    required_context: Vec<String>,
}
```

#### 2.1.3 ConditionValue

Value types supported in conditions.

```rust
pub enum ConditionValue {
    Bool(bool),
    Int(i64),
    Uint(u64),
    Double(f64),
    String(String),
    Duration(std::time::Duration),
    Timestamp(chrono::DateTime<Utc>),
    List(Vec<ConditionValue>),
    Map(HashMap<String, ConditionValue>),
}
```

#### 2.1.4 EdgeState

Complete state held by an edge instance.

```rust
pub struct EdgeState {
    /// Store identifier
    store_id: String,

    /// Authorization model (parsed and validated)
    model: AuthorizationModel,
    model_id: String,

    /// Pre-computed check results
    /// Key: hash of (object, relation, user)
    checks: DashMap<u64, PrecomputedCheck>,

    /// Compiled CEL conditions for evaluation
    conditions: HashMap<String, CompiledCondition>,

    /// Sync watermark (ULID as u128)
    watermark: AtomicU128,

    /// Statistics
    stats: EdgeStats,
}

pub struct EdgeStats {
    total_entries: AtomicU64,
    cache_hits: AtomicU64,
    cache_misses: AtomicU64,
    condition_evaluations: AtomicU64,
}
```

### 2.2 Memory Layout

| Field | Size | Notes |
|-------|------|-------|
| Hash key | 8 bytes | xxhash64 output |
| `allowed_unconditional` | 1 byte | Boolean |
| `conditional_paths` | 0-200 bytes | Typically 0-3 paths |
| `updated_at` | 8 bytes | Unix timestamp |
| **Per-entry overhead** | ~24 bytes | DashMap internal |
| **Total per entry** | 48-256 bytes | Depends on conditions |

### 2.3 Hash Function

```rust
/// Compute hash key for check lookup
/// Uses xxhash64 for speed and good distribution
pub fn compute_check_key(
    store_id: &str,
    object: &str,
    relation: &str,
    user: &str,
) -> u64 {
    let mut hasher = XxHash64::with_seed(0);
    hasher.write(store_id.as_bytes());
    hasher.write(&[0u8]); // separator
    hasher.write(object.as_bytes());
    hasher.write(&[0u8]);
    hasher.write(relation.as_bytes());
    hasher.write(&[0u8]);
    hasher.write(user.as_bytes());
    hasher.finish()
}
```

---

## 3. API Specification

### 3.1 Check API

Edge implements the standard OpenFGA Check API.

#### 3.1.1 gRPC Definition

```protobuf
service OpenFGAService {
    rpc Check(CheckRequest) returns (CheckResponse);
    rpc BatchCheck(BatchCheckRequest) returns (BatchCheckResponse);
}

message CheckRequest {
    string store_id = 1;
    TupleKey tuple_key = 2;
    Struct contextual_tuples = 3;
    string authorization_model_id = 4;
    Struct context = 5;
    ConsistencyPreference consistency = 6;
}

message CheckResponse {
    bool allowed = 1;
    string resolution = 2;
}

enum ConsistencyPreference {
    UNSPECIFIED = 0;
    MINIMIZE_LATENCY = 1;      // Use edge cache
    HIGHER_CONSISTENCY = 2;    // Force central
}
```

#### 3.1.2 HTTP API

```http
POST /stores/{store_id}/check
Content-Type: application/json

{
  "tuple_key": {
    "user": "user:alice",
    "relation": "can_view",
    "object": "document:doc1"
  },
  "authorization_model_id": "01HVMMBCMGZNT3SED4Z17ECXK8",
  "context": {
    "external": false
  }
}
```

#### 3.1.3 Response

```json
{
  "allowed": true,
  "resolution": "edge:hit"
}
```

### 3.2 Edge-Specific Headers

| Header | Values | Description |
|--------|--------|-------------|
| `X-OpenFGA-Consistency` | `eventual`, `strong` | Override consistency |
| `X-OpenFGA-Edge-Hit` | `true`, `false` | Response: was edge cache used |
| `X-OpenFGA-Sync-Lag-Ms` | integer | Current sync lag in ms |

### 3.3 Admin API

Edge exposes an admin API for monitoring and management.

```protobuf
service EdgeAdminService {
    // Get edge statistics
    rpc GetStats(GetStatsRequest) returns (GetStatsResponse);

    // Force sync with central
    rpc ForceSync(ForceSyncRequest) returns (ForceSyncResponse);

    // Health check
    rpc Health(HealthRequest) returns (HealthResponse);

    // Invalidate specific entries
    rpc Invalidate(InvalidateRequest) returns (InvalidateResponse);
}

message GetStatsResponse {
    uint64 total_entries = 1;
    uint64 cache_hits = 2;
    uint64 cache_misses = 3;
    double hit_ratio = 4;
    uint64 memory_bytes = 5;
    string watermark = 6;
    int64 sync_lag_ms = 7;
}
```

---

## 4. Sync Protocol

### 4.1 Message Format

```protobuf
// Published by pre-compute workers, consumed by edge
message CheckResultSync {
    string store_id = 1;
    string model_id = 2;
    repeated CheckResultUpdate updates = 3;
    string watermark = 4;  // ULID
    int64 timestamp = 5;   // Unix timestamp
}

message CheckResultUpdate {
    // Hash key for lookup
    uint64 key_hash = 1;

    // Original tuple components (for debugging/verification)
    string object = 2;
    string relation = 3;
    string user = 4;

    // Pre-computed result
    bool allowed_unconditional = 5;
    repeated ConditionalPath conditions = 6;

    // Deletion marker
    bool deleted = 7;
}

message ConditionalPath {
    string condition_name = 1;
    map<string, google.protobuf.Value> bound_params = 2;
    repeated string required_context = 3;
}
```

### 4.2 Kafka Topic Schema

```yaml
# Topic naming convention
topic: openfga.checks.{store_id}

# Partition key: store_id (all updates for a store go to same partition)
# This ensures ordering within a store

# Message configuration
messages:
  max_size: 1MB
  compression: lz4

# Retention
retention:
  time: 7d
  size: 50GB
```

### 4.3 Sync States

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│  BOOTSTRAP  │────►│   SYNCING   │────►│   HEALTHY   │
└─────────────┘     └─────────────┘     └─────────────┘
       │                   │                   │
       │                   ▼                   ▼
       │            ┌─────────────┐     ┌─────────────┐
       └───────────►│    ERROR    │◄────│    STALE    │
                    └─────────────┘     └─────────────┘
```

| State | Description | Action |
|-------|-------------|--------|
| BOOTSTRAP | Initial load from central | Serve misses only |
| SYNCING | Catching up on stream | Serve with warning |
| HEALTHY | Up to date | Full service |
| STALE | Lag > threshold | Alert, optional fallback |
| ERROR | Sync failed | Circuit breaker |

---

## 5. Pre-Computation Algorithm

### 5.1 Tuple Change Processing

```
Input: TupleChange (write or delete)
Output: Set of CheckResultUpdate messages

Algorithm:
1. Parse tuple: (object, relation, user, condition?)
2. Classify change type:
   - DIRECT: user is entity (user:alice)
   - USERSET: user is userset (group:eng#member)
   - TTU: relation involves tupleset
3. Find affected checks based on type
4. For each affected (object, relation, user):
   a. Compute full check result using model
   b. Create CheckResultUpdate message
5. Batch and publish to Kafka
```

### 5.2 Affected Check Discovery

#### 5.2.1 Direct Assignment

```
Tuple: doc:1#viewer@user:alice

Affected checks:
- (doc:1, viewer, user:alice)
- (doc:1, can_view, user:alice)  // if can_view includes viewer
- (doc:1, can_*, user:alice)     // all computed from viewer
```

#### 5.2.2 Userset Assignment

```
Tuple: doc:1#viewer@team:eng#member

Affected checks:
For each member of team:eng#member:
- (doc:1, viewer, user:X)
- (doc:1, can_view, user:X)
- ...
```

#### 5.2.3 Group Membership Change

```
Tuple: team:eng#member@user:alice

Affected checks:
For each object where team:eng#member has access:
- (object:Y, relation:Z, user:alice)
```

### 5.3 Fan-Out Limits

| Scenario | Max Fan-Out | Strategy |
|----------|-------------|----------|
| Direct tuple | 1-10 | Compute all |
| Group with <1K members | 1K-10K | Compute all |
| Group with >1K members | N/A | Mark as hot, on-demand |
| TTU with <100 children | 100-1K | Compute all |
| TTU with >100 children | N/A | Lazy propagation |

---

## 6. Condition Evaluation

### 6.1 CEL Expression Compilation

```rust
/// Conditions are compiled once when model is loaded
pub struct CompiledCondition {
    /// CEL program
    program: cel::Program,

    /// Parameter declarations
    params: Vec<ConditionParam>,
}

pub struct ConditionParam {
    name: String,
    param_type: ConditionType,
    required: bool,
}
```

### 6.2 Evaluation Algorithm

```rust
impl EdgeState {
    pub fn evaluate_condition(
        &self,
        path: &ConditionalPath,
        request_context: &HashMap<String, Value>,
    ) -> bool {
        // 1. Get compiled condition
        let condition = match self.conditions.get(&path.condition_name) {
            Some(c) => c,
            None => return false,
        };

        // 2. Build evaluation context
        let mut context = HashMap::new();

        // Add bound params from tuple
        for (k, v) in &path.bound_params {
            context.insert(k.clone(), v.clone());
        }

        // Add params from request context
        for key in &path.required_context {
            match request_context.get(key) {
                Some(v) => context.insert(key.clone(), v.clone()),
                None => return false, // Missing required param
            };
        }

        // 3. Evaluate CEL expression
        match condition.program.evaluate(&context) {
            Ok(Value::Bool(result)) => result,
            _ => false,
        }
    }
}
```

### 6.3 Example: external_condition

```
Condition definition:
  condition external_condition (external: bool, allow_external: bool) {
    !external || allow_external
  }

At tuple write time:
  Tuple: space:1#viewer@user:alice with condition(allow_external: true)

  Stored in PrecomputedCheck:
    conditional_paths: [{
      condition_name: "external_condition",
      bound_params: { "allow_external": true },
      required_context: ["external"]
    }]

At check time:
  Request: Check(space:1, can_view, user:alice, context: {external: false})

  Evaluation:
    context = { allow_external: true, external: false }
    result = !false || true = true
    → Allowed
```

---

## 7. Configuration

### 7.1 Edge Configuration

```yaml
# rsfga-config.yaml
rsfga:
  # Store/model subscription
  store_id: "01HVMMBCMGZNT3SED4Z17ECXK8"
  model_id: "01HVMMBCMGZNT3SED4Z17ECXK9"

  # Server settings
  server:
    grpc_port: 8081
    http_port: 8080
    unix_socket: "/tmp/openfga.sock"
    max_connections: 10000

  # Memory limits
  memory:
    max_entries: 10000000  # 10M entries
    max_bytes: 1073741824  # 1GB
    eviction_policy: "lru"

  # Sync settings
  sync:
    protocol: "kafka"
    brokers:
      - "kafka-1:9092"
      - "kafka-2:9092"
    topic: "openfga.checks.{store_id}"
    consumer_group: "edge-{namespace}"
    batch_size: 1000
    poll_interval_ms: 100

  # Consistency
  consistency:
    max_staleness_seconds: 10
    fallback_on_stale: true

  # Central connection
  central:
    url: "openfga.central.svc:8081"
    timeout_ms: 5000
    retry_attempts: 3
    circuit_breaker:
      failure_threshold: 5
      reset_timeout_seconds: 30
```

### 7.2 Pre-Compute Worker Configuration

```yaml
# precompute-config.yaml
precompute:
  # Database connection
  database:
    host: "postgres.central.svc"
    port: 5432
    database: "openfga"
    max_connections: 50

  # CDC settings
  cdc:
    enabled: true
    slot_name: "openfga_cdc"
    publication: "openfga_tuples"

  # Output
  output:
    protocol: "kafka"
    brokers:
      - "kafka-1:9092"
    compression: "lz4"

  # Processing
  processing:
    workers: 8
    batch_size: 100
    max_fan_out: 10000
    hot_path_threshold: 1000
```

---

## 8. Deployment Specification

### 8.1 Resource Requirements

| Component | CPU | Memory | Disk |
|-----------|-----|--------|------|
| Edge Sidecar | 0.1-0.5 cores | 64MB-2GB | None |
| Pre-Compute Worker | 2-8 cores | 4-16GB | None |
| Kafka (per broker) | 2-4 cores | 8-16GB | 500GB-2TB |

### 8.2 Scaling Guidelines

| Metric | Threshold | Action |
|--------|-----------|--------|
| Edge memory > 80% | Warning | Increase limit or enable eviction |
| Edge latency P95 > 0.5ms | Warning | Check for hot entries |
| Edge latency P95 > 1ms | Critical | Scale workers, check sync |
| Sync lag > 5s | Warning | Scale pre-compute workers |
| Sync lag > 30s | Critical | Investigate, failover to central |
| Cache hit ratio < 90% | Warning | Review hot-path configuration |

### 8.3 Network Requirements

| Path | Protocol | Port | TLS |
|------|----------|------|-----|
| App → Edge | gRPC/HTTP | Unix socket or 8080/8081 | Optional |
| Edge → Central | gRPC | 8081 | Required (mTLS) |
| Edge → Kafka | Kafka | 9092 | Required (SASL/TLS) |
| Pre-compute → Postgres | PostgreSQL | 5432 | Required |
| Pre-compute → Kafka | Kafka | 9092 | Required (SASL/TLS) |

---

## 9. Error Codes

### 9.1 Edge-Specific Errors

| Code | Name | Description |
|------|------|-------------|
| EDGE_001 | SYNC_LAG_EXCEEDED | Sync lag exceeds threshold |
| EDGE_002 | CENTRAL_UNAVAILABLE | Cannot reach central server |
| EDGE_003 | MODEL_MISMATCH | Request model_id doesn't match |
| EDGE_004 | CONDITION_EVAL_ERROR | Condition evaluation failed |
| EDGE_005 | MEMORY_LIMIT | Edge memory limit reached |
| EDGE_006 | BOOTSTRAP_IN_PROGRESS | Edge still bootstrapping |

### 9.2 Error Responses

```json
{
  "code": "EDGE_001",
  "message": "Sync lag exceeds threshold (current: 45s, max: 30s)",
  "details": {
    "current_lag_seconds": 45,
    "max_allowed_seconds": 30,
    "last_sync_watermark": "01HVMMBCMGZNT3SED4Z17ECXK8"
  }
}
```

---

## Appendix A: Wire Formats

### A.1 Check Result Binary Format

For maximum efficiency, edge uses a compact binary format internally:

```
┌─────────────────────────────────────────────────────────────────┐
│  Key Hash (8 bytes)  │  Flags (1 byte)  │  Paths (variable)    │
└─────────────────────────────────────────────────────────────────┘

Flags byte:
  Bit 0: allowed_unconditional
  Bit 1: has_conditions
  Bit 2-7: reserved

Paths format:
  [count: u8] [path1] [path2] ...

  Each path:
    [condition_name_len: u8] [condition_name: bytes]
    [bound_params_count: u8] [params...]
    [required_context_count: u8] [context_keys...]
```

### A.2 Sync Message Binary Format

```
┌─────────────────────────────────────────────────────────────────┐
│  Magic (4 bytes)  │  Version (1 byte)  │  Flags (1 byte)       │
├─────────────────────────────────────────────────────────────────┤
│  Store ID Length (2 bytes)  │  Store ID (variable)             │
├─────────────────────────────────────────────────────────────────┤
│  Model ID Length (2 bytes)  │  Model ID (variable)             │
├─────────────────────────────────────────────────────────────────┤
│  Watermark (16 bytes, ULID)                                     │
├─────────────────────────────────────────────────────────────────┤
│  Update Count (4 bytes)                                         │
├─────────────────────────────────────────────────────────────────┤
│  Updates (variable)                                             │
└─────────────────────────────────────────────────────────────────┘

Magic: 0x4F464741 ("OFGA")
Version: 0x01
```

---

## Appendix B: Performance Benchmarks

### B.1 Target Latency Breakdown

| Operation | Target | Measured |
|-----------|--------|----------|
| Hash computation | <10μs | 8μs |
| DashMap lookup | <50μs | 35μs |
| Condition evaluation | <100μs | 80μs |
| Response serialization | <50μs | 40μs |
| **Total (unconditional)** | <200μs | **~150μs** |
| **Total (with condition)** | <400μs | **~350μs** |

### B.2 Throughput Targets

| Scenario | Target | Notes |
|----------|--------|-------|
| Unconditional checks | 500K/s | Per edge instance |
| Conditional checks | 200K/s | With CEL evaluation |
| Cache misses | 10K/s | Forwarded to central |

---

## Appendix C: Compatibility Matrix

| OpenFGA Version | Edge Version | Notes |
|-----------------|--------------|-------|
| 1.5.x | 1.0.x | Full compatibility |
| 1.6.x | 1.1.x | New condition types |
| 2.0.x | 2.0.x | API v2 support |

| FGA CLI Version | Edge Version | Notes |
|-----------------|--------------|-------|
| 0.4.x | 1.0.x | Model/tuple import |
| 0.5.x | 1.1.x | Edge-specific commands |
