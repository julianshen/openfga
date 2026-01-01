# rsfga: Design Document

**Version**: 1.0
**Status**: Draft
**Authors**: Architecture Team
**Last Updated**: 2025-12-31

---

## 1. Executive Summary

This document describes the design of **rsfga** (Rust FGA), a high-performance edge deployment architecture for OpenFGA that achieves **<1ms P95 latency** for Check operations while maintaining full API compatibility with the existing OpenFGA server.

### 1.1 Goals

| Goal | Target | Approach |
|------|--------|----------|
| Check latency P95 | <1ms | Pre-computed results + O(1) lookup |
| Check latency P99 | <2ms | In-memory storage, no disk I/O |
| Throughput per edge | 500K checks/s | Rust implementation, lock-free |
| Memory per edge | 50MB - 2GB | Model-scoped, positive-only storage |
| API compatibility | 100% | Same gRPC/HTTP API as OpenFGA |

### 1.2 Non-Goals

- Edge does not handle writes (proxied to central)
- Edge does not store raw tuples (only pre-computed results)
- Edge does not perform graph traversal (pre-computed at central)
- Edge is not a full database replica

---

## 2. Architecture Overview

### 2.1 System Components

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              KUBERNETES CLUSTER                             │
│                                                                             │
│  ┌───────────────────────────────────────────────────────────────────────┐ │
│  │                    APPLICATION NAMESPACE                              │ │
│  │                                                                       │ │
│  │  ┌─────────────────┐    Unix Socket    ┌─────────────────────────┐   │ │
│  │  │   Application   │◄─────────────────►│   rsfga Edge Sidecar    │   │ │
│  │  │   Container     │     (<0.1ms)      │   (Rust)                │   │ │
│  │  │                 │                   │                         │   │ │
│  │  │  • gRPC Client  │                   │  • Model (10-100KB)     │   │ │
│  │  │  • OpenFGA SDK  │                   │  • Check Cache (HashMap)│   │ │
│  │  └─────────────────┘                   │  • Condition Evaluator  │   │ │
│  │                                        └────────────┬────────────┘   │ │
│  └─────────────────────────────────────────────────────┼─────────────────┘ │
│                                                        │                   │
│                                              Async Sync (Kafka/gRPC)       │
│                                                        │                   │
│  ┌─────────────────────────────────────────────────────▼─────────────────┐ │
│  │                    OPENFGA CENTRAL NAMESPACE                          │ │
│  │                                                                       │ │
│  │  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐       │ │
│  │  │  OpenFGA        │  │  Pre-Compute    │  │  Kafka/NATS     │       │ │
│  │  │  Server         │  │  Workers        │  │  (CDC Stream)   │       │ │
│  │  └────────┬────────┘  └────────┬────────┘  └─────────────────┘       │ │
│  │           │                    │                                      │ │
│  │  ┌────────▼────────────────────▼────────┐                            │ │
│  │  │           PostgreSQL                 │                            │ │
│  │  │     (Full Data - Source of Truth)    │                            │ │
│  │  └──────────────────────────────────────┘                            │ │
│  └───────────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.2 Component Responsibilities

| Component | Responsibility | Technology |
|-----------|---------------|------------|
| **rsfga Edge Sidecar** | Serve Check requests <1ms | Rust + DashMap |
| **Central Server** | Full OpenFGA (Write, Read, Check) | Go (existing) |
| **Pre-Compute Workers** | Transform tuples → check results | Go or Rust |
| **Message Queue** | CDC stream for sync | Kafka/NATS/Pulsar |
| **PostgreSQL** | Source of truth | Existing |

### 2.3 Data Flow

```
WRITE PATH (Async, not latency-critical)
─────────────────────────────────────────
App → Central OpenFGA → PostgreSQL → CDC → Pre-Compute → Kafka → Edge

CHECK PATH (Sync, <1ms target)
──────────────────────────────
App → Edge Sidecar → HashMap.get() → Return
         │
         └─(miss)→ Central OpenFGA → Return + Cache
```

---

## 3. Edge Sidecar Design

### 3.1 Storage Model

Edge stores **pre-computed check results**, not tuples:

```rust
/// What edge stores (per model/store)
pub struct EdgeState {
    /// Authorization model (small, ~10-100KB)
    model: AuthorizationModel,
    model_id: String,

    /// Pre-computed: hash(object, relation, user) → result
    checks: DashMap<u64, PrecomputedCheck>,

    /// Compiled conditions for runtime evaluation
    conditions: HashMap<String, CompiledCondition>,

    /// Sync watermark
    watermark: AtomicU64,
}

/// Pre-computed check result
pub struct PrecomputedCheck {
    /// True if allowed without conditions
    allowed_unconditional: bool,

    /// Conditions that must pass (if any)
    conditional_paths: Vec<ConditionalPath>,
}

pub struct ConditionalPath {
    condition_name: String,
    bound_params: HashMap<String, Value>,  // From tuple
    required_context: Vec<String>,          // From request
}
```

### 3.2 Check Algorithm

```rust
impl EdgeState {
    /// Check operation - target <0.2ms
    pub fn check(&self, req: &CheckRequest) -> CheckResult {
        // Step 1: Hash lookup O(1)
        let key = xxhash64(&req.store_id, &req.object, &req.relation, &req.user);

        match self.checks.get(&key) {
            Some(precomputed) => {
                // Step 2a: Unconditional - return immediately
                if precomputed.allowed_unconditional {
                    return CheckResult::Allowed;
                }

                // Step 2b: No paths - denied
                if precomputed.conditional_paths.is_empty() {
                    return CheckResult::Denied;
                }

                // Step 2c: Evaluate conditions with request context
                for path in &precomputed.conditional_paths {
                    if self.evaluate_condition(path, &req.context) {
                        return CheckResult::Allowed;
                    }
                }
                CheckResult::Denied
            }
            None => CheckResult::Miss,  // Forward to central
        }
    }
}
```

### 3.3 Memory Requirements

| Scale | Users | Objects/User | Allowed Checks | Hot (10%) | Memory |
|-------|-------|--------------|----------------|-----------|--------|
| Small | 1K | 500 | 150K | 15K | **1 MB** |
| Medium | 10K | 1K | 5M | 500K | **24 MB** |
| Large | 100K | 2K | 100M | 10M | **480 MB** |
| Very Large | 1M | 1K | 300M | 30M | **1.4 GB** |

---

## 4. Pre-Computation Engine

### 4.1 Trigger: Tuple Change

When a tuple is written/deleted, the pre-compute worker:

1. **Classifies** the change type
2. **Finds** all affected (object, relation, user) combinations
3. **Computes** each check result using the model
4. **Publishes** results to the sync stream

### 4.2 Change Types and Fan-Out

| Change Type | Example | Affected Checks |
|-------------|---------|-----------------|
| Direct assignment | `doc:1#viewer@alice` | 1 + computed relations |
| Computed userset | `doc:1#editor@alice` | 1 + all relations computed from editor |
| Group membership | `team:eng#member@alice` | 1 + all objects granting access to team |
| TTU (inheritance) | `folder:f1#viewer@alice` | 1 + all children inheriting from folder |

### 4.3 Condition Handling

For tuples with conditions (e.g., `external_condition`):

```
Pre-compute stores:
{
  "allowed_unconditional": false,
  "conditional_paths": [{
    "condition": "external_condition",
    "bound_params": { "allow_external": true },   // From tuple
    "required_context": ["external"]               // From request
  }]
}

At edge runtime:
  evaluate(!external || allow_external) using request context
```

---

## 5. Sync Protocol

### 5.1 Message Format

```protobuf
message CheckResultSync {
    string store_id = 1;
    string model_id = 2;
    repeated CheckResultUpdate updates = 3;
    string watermark = 4;  // ULID for ordering
}

message CheckResultUpdate {
    uint64 key_hash = 1;
    string object = 2;
    string relation = 3;
    string user = 4;
    bool allowed_unconditional = 5;
    repeated ConditionalPath conditions = 6;
    bool deleted = 7;
}

message ConditionalPath {
    string condition_name = 1;
    map<string, google.protobuf.Value> bound_params = 2;
    repeated string required_context = 3;
}
```

### 5.2 Sync Configuration

```yaml
edge:
  subscriptions:
    - store_id: "store_myapp"
      model_id: "model_v3"
      mode: "full"  # or "hot_path"

  sync:
    protocol: "kafka"  # or "grpc_stream"
    topic: "openfga.checks.store_myapp"
    consumer_group: "edge-myapp-namespace"
```

---

## 6. API Compatibility

Edge implements the same OpenFGA gRPC/HTTP API:

| Endpoint | Edge Behavior |
|----------|---------------|
| `Check` | Local lookup, fallback to central |
| `BatchCheck` | Local + batch central for misses |
| `ListObjects` | Forward to central |
| `ListUsers` | Forward to central |
| `Write` | Forward to central |
| `Read` | Forward to central |
| `ReadAuthorizationModel` | Return cached model |

### 6.1 SDK Compatibility

Existing OpenFGA SDKs work unchanged:

```python
# Python SDK - works with edge
from openfga_sdk import OpenFgaClient

client = OpenFgaClient(
    api_url="http://localhost:8080",  # Edge sidecar
    store_id="store_myapp",
)

# Same API, <1ms latency
response = client.check(
    user="user:alice",
    relation="can_view",
    object="document:doc1",
    context={"external": False}
)
```

---

## 7. Deployment Model

### 7.1 Kubernetes Sidecar

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: my-app
spec:
  containers:
  - name: app
    image: my-app:latest
    env:
    - name: OPENFGA_URL
      value: "unix:///tmp/openfga.sock"

  - name: rsfga-edge
    image: rsfga/edge:latest
    env:
    - name: OPENFGA_CENTRAL
      value: "openfga.central.svc:8081"
    - name: OPENFGA_STORE_ID
      value: "store_myapp"
    - name: OPENFGA_MODEL_ID
      value: "model_v3"
    volumeMounts:
    - name: socket
      mountPath: /tmp
    resources:
      limits:
        memory: "512Mi"
        cpu: "500m"

  volumes:
  - name: socket
    emptyDir: {}
```

---

## 8. Consistency Model

### 8.1 Bounded Staleness

Edge provides **bounded staleness** consistency:

- Writes are applied to central immediately
- Pre-computation runs async (typically <5 seconds)
- Edge receives updates via streaming
- Maximum staleness is configurable (default: 10s)

### 8.2 Consistency Guarantees

| Scenario | Guarantee |
|----------|-----------|
| Check after own write | May be stale for ~5-10s |
| Check for others' writes | Eventually consistent |
| Model update | Edge refreshes on model_id change |

### 8.3 Strong Consistency Option

For critical operations, bypass edge:

```python
# Force check at central (strong consistency)
response = client.check(
    user="user:alice",
    relation="admin",
    object="account:billing",
    consistency="strong"  # Header: X-OpenFGA-Consistency: strong
)
```

---

## 9. Failure Modes

### 9.1 Edge Failure

- **Symptom**: Edge sidecar crashes/unavailable
- **Impact**: Check requests fail
- **Mitigation**: Kubernetes restarts sidecar, app retries
- **Recovery**: ~5-10 seconds

### 9.2 Central Failure

- **Symptom**: Central OpenFGA unavailable
- **Impact**: Cache misses fail, writes fail
- **Mitigation**: Edge serves cached results, circuit breaker
- **Recovery**: Edge continues serving hits until central recovers

### 9.3 Sync Lag

- **Symptom**: Edge data stale > threshold
- **Impact**: Stale authorization decisions
- **Mitigation**: Alert, optional fallback to central
- **Recovery**: Sync catches up when stream resumes

---

## 10. Metrics and Observability

### 10.1 Key Metrics

```yaml
# rsfga metrics
rsfga_check_duration_seconds:
  type: histogram
  labels: [result]  # hit, miss, error

rsfga_cache_hit_ratio:
  type: gauge

rsfga_sync_lag_seconds:
  type: gauge

rsfga_entries_total:
  type: gauge

rsfga_memory_bytes:
  type: gauge
```

### 10.2 Alerts

```yaml
alerts:
  - name: RsfgaLatencyHigh
    expr: histogram_quantile(0.95, rsfga_check_duration_seconds) > 0.001
    severity: warning

  - name: RsfgaCacheHitLow
    expr: rsfga_cache_hit_ratio < 0.90
    severity: warning

  - name: RsfgaSyncLagHigh
    expr: rsfga_sync_lag_seconds > 30
    severity: critical
```

---

## 11. Security Considerations

### 11.1 Transport Security

- Edge ↔ App: Unix socket (same pod, trusted)
- Edge ↔ Central: mTLS required
- Edge ↔ Kafka: SASL/TLS

### 11.2 Data at Rest

- Edge stores only hashes and boolean results
- No PII in edge storage
- Memory-only (no disk persistence)

### 11.3 Access Control

- Edge sidecar runs with minimal privileges
- No write access to central
- Store/model scoped subscriptions

---

## 12. Future Enhancements

| Enhancement | Priority | Description |
|-------------|----------|-------------|
| Hot-path learning | P1 | Auto-detect frequently checked permissions |
| Negative caching | P1 | Cache denied results with bloom filter |
| Prefetching | P2 | Predictive loading of related permissions |
| Multi-model edge | P2 | Single edge serving multiple models |
| Edge-to-edge sync | P3 | Peer sync for global deployments |

---

## Appendix A: Glossary

| Term | Definition |
|------|------------|
| rsfga | Rust FGA - high-performance edge deployment for OpenFGA |
| Edge | Sidecar container serving Check requests |
| Central | Full OpenFGA server with database |
| Pre-compute | Process converting tuples to check results |
| Check result | Pre-computed (object, relation, user) → allowed |
| Condition | Runtime expression evaluated at edge |
| Bounded staleness | Consistency model with max lag guarantee |

---

## Appendix B: Related Documents

- [02_SPEC_DOCUMENT.md](02_SPEC_DOCUMENT.md) - Detailed specifications
- [03_TEST_DOCUMENT.md](03_TEST_DOCUMENT.md) - Test cases
- [04_REFERENCE_DOCUMENT.md](04_REFERENCE_DOCUMENT.md) - Research and references
