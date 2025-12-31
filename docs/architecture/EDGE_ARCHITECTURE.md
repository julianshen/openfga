# OpenFGA Edge Architecture: Detailed Design

## 1. Complete System Architecture

```
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│                                    CLIENT LAYER                                         │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐   │
│  │ Product A   │  │ Product B   │  │ Product C   │  │ Mobile Apps │  │ IoT Devices │   │
│  │ (SaaS)      │  │ (Gaming)    │  │ (Finance)   │  │             │  │             │   │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘   │
└─────────┼────────────────┼────────────────┼────────────────┼────────────────┼──────────┘
          │                │                │                │                │
          ▼                ▼                ▼                ▼                ▼
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│                              GLOBAL LOAD BALANCER                                       │
│  ┌─────────────────────────────────────────────────────────────────────────────────┐   │
│  │  • GeoDNS routing (Cloudflare/AWS Route53)                                      │   │
│  │  • Product-aware routing (X-Product-ID header)                                  │   │
│  │  • Latency-based selection                                                      │   │
│  │  • Health-check failover                                                        │   │
│  └─────────────────────────────────────────────────────────────────────────────────┘   │
└───────────────────────────────────────┬─────────────────────────────────────────────────┘
                                        │
        ┌───────────────────────────────┼───────────────────────────────┐
        │                               │                               │
        ▼                               ▼                               ▼
┌───────────────────────┐  ┌───────────────────────┐  ┌───────────────────────┐
│     EDGE CLUSTER      │  │     EDGE CLUSTER      │  │     EDGE CLUSTER      │
│     Asia-Pacific      │  │     Europe            │  │     Americas          │
│  ┌─────────────────┐  │  │  ┌─────────────────┐  │  │  ┌─────────────────┐  │
│  │ Tokyo  Singapore│  │  │  │ London Frankfurt│  │  │  │ Virginia SaoPaulo│ │
│  │ Sydney Mumbai   │  │  │  │ Paris  Amsterdam│  │  │  │ Oregon   Toronto │ │
│  └────────┬────────┘  │  │  └────────┬────────┘  │  │  └────────┬────────┘  │
└───────────┼───────────┘  └───────────┼───────────┘  └───────────┼───────────┘
            │                          │                          │
            └──────────────────────────┼──────────────────────────┘
                                       │
                                       ▼
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│                              REGIONAL LAYER                                             │
│  ┌─────────────────────────┐ ┌─────────────────────────┐ ┌─────────────────────────┐   │
│  │    APAC Regional        │ │    EMEA Regional        │ │    Americas Regional    │   │
│  │  ┌───────────────────┐  │ │  ┌───────────────────┐  │ │  ┌───────────────────┐  │   │
│  │  │ OpenFGA Cluster   │  │ │  │ OpenFGA Cluster   │  │ │  │ OpenFGA Cluster   │  │   │
│  │  │ (3 replicas)      │  │ │  │ (3 replicas)      │  │ │  │ (3 replicas)      │  │   │
│  │  └─────────┬─────────┘  │ │  └─────────┬─────────┘  │ │  └─────────┬─────────┘  │   │
│  │  ┌─────────▼─────────┐  │ │  ┌─────────▼─────────┐  │ │  ┌─────────▼─────────┐  │   │
│  │  │ Valkey Cluster    │  │ │  │ Valkey Cluster    │  │ │  │ Valkey Cluster    │  │   │
│  │  │ (L2 Cache)        │  │ │  │ (L2 Cache)        │  │ │  │ (L2 Cache)        │  │   │
│  │  └─────────┬─────────┘  │ │  └─────────┬─────────┘  │ │  └─────────┬─────────┘  │   │
│  │  ┌─────────▼─────────┐  │ │  ┌─────────▼─────────┐  │ │  ┌─────────▼─────────┐  │   │
│  │  │ PostgreSQL        │  │ │  │ PostgreSQL        │  │ │  │ PostgreSQL        │  │   │
│  │  │ Read Replica      │  │ │  │ Read Replica      │  │ │  │ Read Replica      │  │   │
│  │  └───────────────────┘  │ │  └───────────────────┘  │ │  └───────────────────┘  │   │
│  └─────────────────────────┘ └─────────────────────────┘ └─────────────────────────┘   │
└───────────────────────────────────────┬─────────────────────────────────────────────────┘
                                        │
                    ┌───────────────────┼───────────────────┐
                    │                   │                   │
                    ▼                   ▼                   ▼
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│                              CENTRAL LAYER                                              │
│  ┌─────────────────────────────────────────────────────────────────────────────────┐   │
│  │                         Global Coordination                                     │   │
│  │  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐                 │   │
│  │  │ Control Plane   │  │ Schema Registry │  │ Metrics Agg     │                 │   │
│  │  │ (Product Config)│  │ (Auth Models)   │  │ (Prometheus)    │                 │   │
│  │  └─────────────────┘  └─────────────────┘  └─────────────────┘                 │   │
│  └─────────────────────────────────────────────────────────────────────────────────┘   │
│  ┌─────────────────────────────────────────────────────────────────────────────────┐   │
│  │                         Primary Database Cluster                                │   │
│  │  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐    │   │
│  │  │   Primary     │  │   Sync        │  │   Sync        │  │   Async       │    │   │
│  │  │   (Writes)    │  │   Replica     │  │   Replica     │  │   Replica     │    │   │
│  │  │   us-east-1   │  │   us-west-2   │  │   eu-west-1   │  │   ap-south-1  │    │   │
│  │  └───────────────┘  └───────────────┘  └───────────────┘  └───────────────┘    │   │
│  └─────────────────────────────────────────────────────────────────────────────────┘   │
│  ┌─────────────────────────────────────────────────────────────────────────────────┐   │
│  │                         Event Streaming (Kafka/Pulsar)                          │   │
│  │  ┌─────────────────────────────────────────────────────────────────────────┐   │   │
│  │  │  Topics: tuple-changes, model-updates, cache-invalidation, audit-log   │   │   │
│  │  └─────────────────────────────────────────────────────────────────────────┘   │   │
│  └─────────────────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────────────────┘
```

---

## 2. Edge Node Detail Architecture

```
┌─────────────────────────────────────────────────────────────────────────────────────┐
│                            EDGE NODE (per PoP)                                      │
│  ┌───────────────────────────────────────────────────────────────────────────────┐  │
│  │                              Envoy Proxy                                      │  │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐          │  │
│  │  │ Rate Limit  │  │ Auth Cache  │  │ Circuit     │  │ Request     │          │  │
│  │  │ (per-tenant)│  │ (JWT/OIDC)  │  │ Breaker     │  │ Routing     │          │  │
│  │  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘          │  │
│  └───────────────────────────────────────┬───────────────────────────────────────┘  │
│                                          │                                          │
│  ┌───────────────────────────────────────▼───────────────────────────────────────┐  │
│  │                         OpenFGA Edge Instance                                 │  │
│  │                                                                               │  │
│  │  ┌─────────────────────────────────────────────────────────────────────────┐ │  │
│  │  │                        Request Router                                   │ │  │
│  │  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐    │ │  │
│  │  │  │ Check       │  │ BatchCheck  │  │ Read        │  │ Write       │    │ │  │
│  │  │  │ (local+fwd) │  │ (partition) │  │ (local)     │  │ (proxy)     │    │ │  │
│  │  │  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘    │ │  │
│  │  └─────────┼────────────────┼────────────────┼────────────────┼───────────┘ │  │
│  │            │                │                │                │             │  │
│  │  ┌─────────▼────────────────▼────────────────▼────────────────▼───────────┐ │  │
│  │  │                      Resolution Engine                                 │ │  │
│  │  │  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐        │ │  │
│  │  │  │ Simple Resolver │  │ Cached Resolver │  │ Forward to      │        │ │  │
│  │  │  │ (direct tuples) │  │ (L1 + L2 cache) │  │ Regional        │        │ │  │
│  │  │  └─────────────────┘  └─────────────────┘  └─────────────────┘        │ │  │
│  │  └────────────────────────────────┬───────────────────────────────────────┘ │  │
│  │                                   │                                         │  │
│  │  ┌────────────────────────────────▼───────────────────────────────────────┐ │  │
│  │  │                        Local Storage                                   │ │  │
│  │  │                                                                        │ │  │
│  │  │  ┌──────────────────────────────────────────────────────────────────┐ │ │  │
│  │  │  │                    Product Partition Manager                     │ │ │  │
│  │  │  │                                                                  │ │ │  │
│  │  │  │  ┌────────────┐  ┌────────────┐  ┌────────────┐  ┌────────────┐ │ │ │  │
│  │  │  │  │ Product A  │  │ Product B  │  │ Product C  │  │ Product D  │ │ │ │  │
│  │  │  │  │ Store      │  │ Store      │  │ (not here) │  │ Store      │ │ │ │  │
│  │  │  │  │            │  │            │  │            │  │            │ │ │ │  │
│  │  │  │  │ • Tuples   │  │ • Tuples   │  │ Forward to │  │ • Tuples   │ │ │ │  │
│  │  │  │  │ • Models   │  │ • Models   │  │ Regional   │  │ • Models   │ │ │ │  │
│  │  │  │  │ • Cache    │  │ • Cache    │  │            │  │ • Cache    │ │ │ │  │
│  │  │  │  └────────────┘  └────────────┘  └────────────┘  └────────────┘ │ │ │  │
│  │  │  └──────────────────────────────────────────────────────────────────┘ │ │  │
│  │  │                                                                        │ │  │
│  │  │  ┌──────────────────────────────────────────────────────────────────┐ │ │  │
│  │  │  │                    Embedded Storage (sled/RocksDB)               │ │ │  │
│  │  │  │  ┌────────────┐  ┌────────────┐  ┌────────────┐  ┌────────────┐ │ │ │  │
│  │  │  │  │ Tuple DB   │  │ Model DB   │  │ Cache DB   │  │ Sync State │ │ │ │  │
│  │  │  │  └────────────┘  └────────────┘  └────────────┘  └────────────┘ │ │ │  │
│  │  │  └──────────────────────────────────────────────────────────────────┘ │ │  │
│  │  └────────────────────────────────────────────────────────────────────────┘ │  │
│  └───────────────────────────────────────────────────────────────────────────────┘  │
│                                                                                     │
│  ┌───────────────────────────────────────────────────────────────────────────────┐  │
│  │                         Sync Agent                                            │  │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐          │  │
│  │  │ Kafka       │  │ Product     │  │ Watermark   │  │ Health      │          │  │
│  │  │ Consumer    │  │ Filter      │  │ Tracker     │  │ Reporter    │          │  │
│  │  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘          │  │
│  └───────────────────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────────────────┘
```

---

## 3. Data Partitioning Strategy: Product-Based vs Region-Based

### 3.1 Why NOT Full Replication?

| Approach | Storage Cost | Sync Bandwidth | Consistency Lag | Complexity |
|----------|--------------|----------------|-----------------|------------|
| Full Copy | 100% × N edges | Very High | High | Low |
| Region-Based | 30% × N edges | Medium | Medium | Medium |
| **Product-Based** | **5-15% × N edges** | **Low** | **Low** | **Medium** |
| Hybrid (Recommended) | 10-20% × N edges | Low-Medium | Low | High |

### 3.2 Product-Based Partitioning Model

```
┌─────────────────────────────────────────────────────────────────────────────────────┐
│                        PRODUCT AFFINITY MATRIX                                      │
│                                                                                     │
│  Product/Edge │ Tokyo │ Sydney │ London │ Frankfurt │ Virginia │ Oregon │ SaoPaulo │
│  ─────────────┼───────┼────────┼────────┼───────────┼──────────┼────────┼──────────│
│  Product A    │  ●●●  │   ●●   │   ●    │     ●     │    ●●    │   ●    │    ●     │
│  (APAC Focus) │       │        │        │           │          │        │          │
│  ─────────────┼───────┼────────┼────────┼───────────┼──────────┼────────┼──────────│
│  Product B    │   ●   │   ●    │  ●●●   │    ●●●    │    ●●    │   ●    │    ●     │
│  (EU Focus)   │       │        │        │           │          │        │          │
│  ─────────────┼───────┼────────┼────────┼───────────┼──────────┼────────┼──────────│
│  Product C    │   ○   │   ○    │   ○    │     ○     │   ●●●    │  ●●●   │   ●●●    │
│  (Americas)   │       │        │        │           │          │        │          │
│  ─────────────┼───────┼────────┼────────┼───────────┼──────────┼────────┼──────────│
│  Product D    │  ●●●  │  ●●●   │  ●●●   │    ●●●    │   ●●●    │  ●●●   │   ●●●    │
│  (Global)     │       │        │        │           │          │        │          │
│  ─────────────┴───────┴────────┴────────┴───────────┴──────────┴────────┴──────────│
│                                                                                     │
│  Legend: ●●● = Full data    ●● = Hot data only    ● = Cache only    ○ = Not present│
└─────────────────────────────────────────────────────────────────────────────────────┘
```

### 3.3 Data Tier Classification

```
┌─────────────────────────────────────────────────────────────────────────────────────┐
│                              DATA CLASSIFICATION                                    │
│                                                                                     │
│  ┌─────────────────────────────────────────────────────────────────────────────┐   │
│  │  TIER 1: Always Replicate (Critical)                                       │   │
│  │  ─────────────────────────────────────────────────────────────────────────  │   │
│  │  • Authorization Models (small, essential for resolution)                   │   │
│  │  • Store metadata                                                           │   │
│  │  • Product configuration                                                    │   │
│  │  • Admin user permissions                                                   │   │
│  │                                                                             │   │
│  │  Size: ~1% of total data                                                    │   │
│  │  Sync: Real-time (Kafka streaming)                                          │   │
│  └─────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                     │
│  ┌─────────────────────────────────────────────────────────────────────────────┐   │
│  │  TIER 2: Product-Affinity Replicate (Hot Data)                              │   │
│  │  ─────────────────────────────────────────────────────────────────────────  │   │
│  │  • Tuples for products with affinity to this edge                           │   │
│  │  • Recently accessed tuples (LRU-based)                                     │   │
│  │  • Tuples for users geolocated to this region                               │   │
│  │                                                                             │   │
│  │  Size: 10-30% of total data per edge                                        │   │
│  │  Sync: Near real-time (5-30 second lag)                                     │   │
│  └─────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                     │
│  ┌─────────────────────────────────────────────────────────────────────────────┐   │
│  │  TIER 3: On-Demand Cache (Warm Data)                                        │   │
│  │  ─────────────────────────────────────────────────────────────────────────  │   │
│  │  • Tuples fetched on cache miss                                             │   │
│  │  • TTL-based expiration (30s - 5min)                                        │   │
│  │  • Check results (allowed/denied)                                           │   │
│  │                                                                             │   │
│  │  Size: Variable (bounded by cache size)                                     │   │
│  │  Sync: Pull on demand                                                       │   │
│  └─────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                     │
│  ┌─────────────────────────────────────────────────────────────────────────────┐   │
│  │  TIER 4: Never at Edge (Cold/Sensitive)                                     │   │
│  │  ─────────────────────────────────────────────────────────────────────────  │   │
│  │  • Audit logs and changelog                                                 │   │
│  │  • Assertions (test data)                                                   │   │
│  │  • Products not deployed to this region                                     │   │
│  │  • Compliance-restricted data (GDPR, data residency)                        │   │
│  │                                                                             │   │
│  │  Size: 0% at edge                                                           │   │
│  │  Sync: Always forward to regional/central                                   │   │
│  └─────────────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────────────┘
```

---

## 4. Product-Based Routing Configuration

### 4.1 Product Registry

```yaml
# product-registry.yaml (stored in Central Control Plane)
products:
  product-a:
    id: "prod_abc123"
    name: "Enterprise SaaS"
    stores:
      - "store_ent_prod"
      - "store_ent_staging"

    # Edge affinity configuration
    edge_affinity:
      primary_regions: ["apac"]
      secondary_regions: ["americas"]
      excluded_regions: []  # Can access via regional

    # Data replication rules
    replication:
      tier1_always:
        - object_type: "organization"
        - object_type: "team"
      tier2_hot:
        - object_type: "document"
          conditions:
            accessed_within: "24h"
            min_access_count: 5
        - object_type: "folder"
      tier3_cache_only:
        - object_type: "comment"
        - object_type: "attachment"

    # Consistency requirements
    consistency:
      default: "bounded_staleness"
      max_staleness: "10s"
      critical_objects:
        - pattern: "organization:*#admin"
          level: "strong"

  product-b:
    id: "prod_def456"
    name: "Gaming Platform"
    stores:
      - "store_game_prod"

    edge_affinity:
      primary_regions: ["emea", "americas"]
      secondary_regions: ["apac"]

    replication:
      tier1_always:
        - object_type: "game"
        - object_type: "guild"
      tier2_hot:
        - object_type: "player"
          conditions:
            # Replicate players who logged in recently
            accessed_within: "1h"
        - object_type: "match"
          conditions:
            # Only active matches
            created_within: "2h"

    consistency:
      default: "eventual"
      max_staleness: "30s"
      # Gaming can tolerate more staleness

  product-c:
    id: "prod_ghi789"
    name: "Financial Services"
    stores:
      - "store_fin_prod"

    edge_affinity:
      primary_regions: ["americas"]
      secondary_regions: []  # No secondary - compliance
      excluded_regions: ["apac"]  # Data residency requirement

    replication:
      tier1_always:
        - object_type: "account"
        - object_type: "portfolio"
      tier2_hot: []  # Nothing hot at edge

    consistency:
      default: "strong"  # Always consistent
      # Financial requires strong consistency

    compliance:
      data_residency: "us-only"
      audit_required: true
```

### 4.2 Edge Configuration

```yaml
# edge-config-tokyo.yaml
edge:
  id: "edge-tokyo-1"
  region: "apac"
  location: "ap-northeast-1"

  # Products hosted at this edge
  products:
    - product_id: "prod_abc123"  # Product A - primary
      mode: "full"
      replication_lag_max: "5s"

    - product_id: "prod_def456"  # Product B - secondary
      mode: "hot_only"
      replication_lag_max: "30s"

    - product_id: "prod_ghi789"  # Product C - NOT HERE
      mode: "forward"  # Always forward to regional

  # Storage limits
  storage:
    max_tuples: 10_000_000
    max_models: 1000
    cache_size_mb: 512

  # Per-product quotas
  quotas:
    prod_abc123:
      max_tuples: 5_000_000
      cache_mb: 256
    prod_def456:
      max_tuples: 3_000_000
      cache_mb: 128
```

---

## 5. Request Routing Flow

```
┌─────────────────────────────────────────────────────────────────────────────────────┐
│                              REQUEST ROUTING DECISION TREE                          │
│                                                                                     │
│  Incoming Request                                                                   │
│       │                                                                             │
│       ▼                                                                             │
│  ┌─────────────────────────┐                                                        │
│  │ Extract Product ID      │                                                        │
│  │ (from X-Product-ID or   │                                                        │
│  │  store_id mapping)      │                                                        │
│  └───────────┬─────────────┘                                                        │
│              │                                                                      │
│              ▼                                                                      │
│  ┌─────────────────────────┐     No      ┌─────────────────────────┐               │
│  │ Is product configured   │────────────▶│ Forward to Regional     │               │
│  │ for this edge?          │             │ (unknown product)       │               │
│  └───────────┬─────────────┘             └─────────────────────────┘               │
│              │ Yes                                                                  │
│              ▼                                                                      │
│  ┌─────────────────────────┐                                                        │
│  │ Product Mode?           │                                                        │
│  └───────────┬─────────────┘                                                        │
│              │                                                                      │
│    ┌─────────┼─────────┬──────────────┐                                            │
│    │         │         │              │                                            │
│    ▼         ▼         ▼              ▼                                            │
│ ┌──────┐ ┌──────┐ ┌──────┐    ┌───────────┐                                        │
│ │ full │ │ hot  │ │cache │    │  forward  │                                        │
│ │      │ │ only │ │ only │    │           │                                        │
│ └──┬───┘ └──┬───┘ └──┬───┘    └─────┬─────┘                                        │
│    │        │        │              │                                              │
│    ▼        ▼        ▼              ▼                                              │
│  ┌─────────────────────────────────────────────────────────────────────────────┐   │
│  │                         RESOLUTION PATH                                     │   │
│  │                                                                             │   │
│  │  full mode:                                                                 │   │
│  │    1. Check L1 cache                                                        │   │
│  │    2. Check local tuple store                                               │   │
│  │    3. Resolve locally (full graph traversal)                                │   │
│  │    4. If missing data → fetch from regional → cache locally                 │   │
│  │                                                                             │   │
│  │  hot_only mode:                                                             │   │
│  │    1. Check L1 cache                                                        │   │
│  │    2. Check local tuple store (hot data only)                               │   │
│  │    3. If found → resolve locally (simple cases)                             │   │
│  │    4. If complex/missing → forward to regional                              │   │
│  │                                                                             │   │
│  │  cache_only mode:                                                           │   │
│  │    1. Check L1 cache (check results only)                                   │   │
│  │    2. If hit → return cached result                                         │   │
│  │    3. If miss → forward to regional → cache result                          │   │
│  │                                                                             │   │
│  │  forward mode:                                                              │   │
│  │    1. Immediately forward to regional                                       │   │
│  │    2. No local storage/caching                                              │   │
│  │                                                                             │   │
│  └─────────────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────────────┘
```

---

## 6. Selective Sync Implementation

```rust
// Product-based sync filter
pub struct ProductSyncFilter {
    product_id: ProductId,
    edge_id: EdgeId,
    config: ProductReplicationConfig,
}

impl ProductSyncFilter {
    /// Determines if a tuple change should be synced to this edge
    pub fn should_sync(&self, change: &TupleChange) -> SyncDecision {
        // Tier 1: Always sync critical data
        if self.is_tier1_object(&change.tuple) {
            return SyncDecision::Sync { priority: Priority::High };
        }

        // Check if product is configured for this edge
        let product_mode = self.config.get_mode_for_edge(self.edge_id);

        match product_mode {
            EdgeMode::Full => {
                // Tier 2: Sync hot data based on rules
                if self.matches_tier2_rules(&change.tuple) {
                    SyncDecision::Sync { priority: Priority::Normal }
                } else {
                    // Tier 3: Will be fetched on demand
                    SyncDecision::Skip
                }
            }

            EdgeMode::HotOnly => {
                // Only sync if matches hot data criteria
                if self.is_hot_data(&change.tuple) {
                    SyncDecision::Sync { priority: Priority::Normal }
                } else {
                    SyncDecision::Skip
                }
            }

            EdgeMode::CacheOnly | EdgeMode::Forward => {
                // Never proactively sync
                SyncDecision::Skip
            }
        }
    }

    fn is_tier1_object(&self, tuple: &Tuple) -> bool {
        self.config.tier1_always.iter().any(|rule| {
            rule.matches_object_type(&tuple.object_type)
        })
    }

    fn matches_tier2_rules(&self, tuple: &Tuple) -> bool {
        self.config.tier2_hot.iter().any(|rule| {
            rule.matches(tuple)
        })
    }

    fn is_hot_data(&self, tuple: &Tuple) -> bool {
        // Check access patterns
        let access_stats = self.access_tracker.get_stats(&tuple.key());

        access_stats.map(|stats| {
            stats.access_count >= self.config.min_access_count &&
            stats.last_access.elapsed() < self.config.hot_window
        }).unwrap_or(false)
    }
}

// Sync agent that runs on each edge
pub struct EdgeSyncAgent {
    edge_id: EdgeId,
    products: HashMap<ProductId, ProductSyncFilter>,
    kafka_consumer: KafkaConsumer,
    local_storage: EdgeStorage,
}

impl EdgeSyncAgent {
    pub async fn run(&mut self) {
        loop {
            // Consume from Kafka topic partitioned by store_id
            let batch = self.kafka_consumer.poll(Duration::from_millis(100)).await;

            for change in batch {
                let product_id = self.get_product_for_store(&change.store_id);

                if let Some(filter) = self.products.get(&product_id) {
                    match filter.should_sync(&change) {
                        SyncDecision::Sync { priority } => {
                            self.apply_change(&change, priority).await;
                        }
                        SyncDecision::Skip => {
                            // Just update watermark, don't store
                            self.update_watermark(&change.store_id, &change.ulid);
                        }
                    }
                }
            }
        }
    }

    async fn apply_change(&mut self, change: &TupleChange, priority: Priority) {
        match change.operation {
            Operation::Write => {
                self.local_storage.write_tuple(&change.tuple).await?;
            }
            Operation::Delete => {
                self.local_storage.delete_tuple(&change.tuple.key()).await?;
            }
        }

        // Invalidate related caches
        self.invalidate_caches(&change.tuple).await;

        // Update sync watermark
        self.update_watermark(&change.store_id, &change.ulid);
    }
}
```

---

## 7. Hybrid Approach: Product + Geographic Hints

```
┌─────────────────────────────────────────────────────────────────────────────────────┐
│                           SMART REPLICATION DECISION                                │
│                                                                                     │
│  ┌─────────────────────────────────────────────────────────────────────────────┐   │
│  │                                                                             │   │
│  │   Replication Score = Σ (Factor × Weight)                                   │   │
│  │                                                                             │   │
│  │   ┌─────────────────────────┬────────┬─────────────────────────────────┐   │   │
│  │   │ Factor                  │ Weight │ Description                     │   │   │
│  │   ├─────────────────────────┼────────┼─────────────────────────────────┤   │   │
│  │   │ Product Affinity        │  0.40  │ Is product primary for edge?   │   │   │
│  │   │ Access Frequency        │  0.25  │ How often accessed from edge?  │   │   │
│  │   │ User Geolocation        │  0.15  │ Are users near this edge?      │   │   │
│  │   │ Object Type Priority    │  0.10  │ Is object type critical?       │   │   │
│  │   │ Recency                 │  0.10  │ Recently created/modified?     │   │   │
│  │   └─────────────────────────┴────────┴─────────────────────────────────┘   │   │
│  │                                                                             │   │
│  │   Decision:                                                                 │   │
│  │     Score > 0.8  → Tier 2 (proactive sync)                                 │   │
│  │     Score > 0.5  → Tier 3 (cache on access)                                │   │
│  │     Score < 0.5  → Forward to regional                                     │   │
│  │                                                                             │   │
│  └─────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                     │
│  Example Calculation for tuple "document:doc123#viewer@user:alice":                 │
│                                                                                     │
│  Edge: Tokyo                                                                        │
│  ┌──────────────────────────────────────────────────────────────────────────────┐  │
│  │ • Product Affinity: 1.0 (Product A is primary for APAC)     × 0.40 = 0.40   │  │
│  │ • Access Frequency: 0.8 (accessed 50 times in 24h)          × 0.25 = 0.20   │  │
│  │ • User Geolocation: 0.9 (alice is in Japan)                 × 0.15 = 0.135  │  │
│  │ • Object Type:      0.7 (document is tier2)                 × 0.10 = 0.07   │  │
│  │ • Recency:          0.6 (modified 2 hours ago)              × 0.10 = 0.06   │  │
│  │ ────────────────────────────────────────────────────────────────────────    │  │
│  │ Total Score: 0.865 → Tier 2 (proactive sync to Tokyo)                       │  │
│  └──────────────────────────────────────────────────────────────────────────────┘  │
│                                                                                     │
│  Edge: London                                                                       │
│  ┌──────────────────────────────────────────────────────────────────────────────┐  │
│  │ • Product Affinity: 0.3 (Product A is secondary for EMEA)   × 0.40 = 0.12   │  │
│  │ • Access Frequency: 0.1 (accessed 2 times in 24h)           × 0.25 = 0.025  │  │
│  │ • User Geolocation: 0.0 (alice is not in Europe)            × 0.15 = 0.00   │  │
│  │ • Object Type:      0.7 (document is tier2)                 × 0.10 = 0.07   │  │
│  │ • Recency:          0.6 (modified 2 hours ago)              × 0.10 = 0.06   │  │
│  │ ────────────────────────────────────────────────────────────────────────    │  │
│  │ Total Score: 0.275 → Forward (not replicated to London)                     │  │
│  └──────────────────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────────────────┘
```

---

## 8. Data Volume Estimation

```
┌─────────────────────────────────────────────────────────────────────────────────────┐
│                           STORAGE REQUIREMENTS EXAMPLE                              │
│                                                                                     │
│  Scenario: 100M total tuples, 50 products, 20 edges                                │
│                                                                                     │
│  ┌─────────────────────────────────────────────────────────────────────────────┐   │
│  │                        CENTRAL DATABASE                                     │   │
│  │  ────────────────────────────────────────────────────────────────────────   │   │
│  │  Total Tuples:           100,000,000                                        │   │
│  │  Authorization Models:   500                                                │   │
│  │  Stores:                 200                                                │   │
│  │  Changelog entries:      1,000,000,000 (last 90 days)                       │   │
│  │  ────────────────────────────────────────────────────────────────────────   │   │
│  │  Storage: ~500 GB                                                           │   │
│  └─────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                     │
│  ┌─────────────────────────────────────────────────────────────────────────────┐   │
│  │                        REGIONAL DATABASE (×3)                               │   │
│  │  ────────────────────────────────────────────────────────────────────────   │   │
│  │  Read Replica:           100,000,000 tuples (full)                          │   │
│  │  L2 Cache (Valkey):      10,000,000 check results                           │   │
│  │  ────────────────────────────────────────────────────────────────────────   │   │
│  │  Storage: ~500 GB + 20 GB cache                                             │   │
│  └─────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                     │
│  ┌─────────────────────────────────────────────────────────────────────────────┐   │
│  │                        EDGE NODE (×20)                                      │   │
│  │  ────────────────────────────────────────────────────────────────────────   │   │
│  │                                                                             │   │
│  │  With Full Replication (BAD):                                               │   │
│  │    100M tuples × 20 edges = 2B tuple copies                                 │   │
│  │    Storage: 500 GB × 20 = 10 TB total                                       │   │
│  │                                                                             │   │
│  │  With Product-Based (GOOD):                                                 │   │
│  │    Tier 1 (models, config):     500 KB per edge                             │   │
│  │    Tier 2 (hot tuples):         5-10M tuples per edge (avg)                 │   │
│  │    Tier 3 (cache):              1M cached results per edge                  │   │
│  │    ────────────────────────────────────────────────────────────────────     │   │
│  │    Storage per edge: 25-50 GB                                               │   │
│  │    Total: 500 GB - 1 TB (10-20× reduction)                                  │   │
│  │                                                                             │   │
│  └─────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                     │
│  ┌─────────────────────────────────────────────────────────────────────────────┐   │
│  │                        BANDWIDTH SAVINGS                                    │   │
│  │  ────────────────────────────────────────────────────────────────────────   │   │
│  │                                                                             │   │
│  │  Write rate: 10,000 tuples/second                                           │   │
│  │  Avg tuple size: 500 bytes                                                  │   │
│  │                                                                             │   │
│  │  Full Replication:                                                          │   │
│  │    10,000 × 500 × 20 edges = 100 MB/s = 8.6 TB/day                         │   │
│  │                                                                             │   │
│  │  Product-Based (10% replication):                                           │   │
│  │    10,000 × 500 × 2 edges avg = 10 MB/s = 864 GB/day                       │   │
│  │                                                                             │   │
│  │  Savings: 90% bandwidth reduction                                           │   │
│  │                                                                             │   │
│  └─────────────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────────────┘
```

---

## 9. Deployment Summary

```
┌─────────────────────────────────────────────────────────────────────────────────────┐
│                           RECOMMENDED DEPLOYMENT                                    │
│                                                                                     │
│  ┌─────────────────────────────────────────────────────────────────────────────┐   │
│  │                                                                             │   │
│  │    EDGE (20 PoPs)              REGIONAL (3)           CENTRAL (1)           │   │
│  │    ──────────────              ────────────           ──────────            │   │
│  │                                                                             │   │
│  │    ┌───────────┐               ┌───────────┐          ┌───────────┐         │   │
│  │    │ Instances │               │ Instances │          │ Instances │         │   │
│  │    │ 2 per PoP │               │ 3 per     │          │ 5         │         │   │
│  │    │ = 40 total│               │ = 9 total │          │           │         │   │
│  │    └───────────┘               └───────────┘          └───────────┘         │   │
│  │                                                                             │   │
│  │    ┌───────────┐               ┌───────────┐          ┌───────────┐         │   │
│  │    │ Storage   │               │ Storage   │          │ Storage   │         │   │
│  │    │ 50GB each │               │ 500GB     │          │ 1TB+      │         │   │
│  │    │ embedded  │               │ PostgreSQL│          │ PostgreSQL│         │   │
│  │    └───────────┘               │ + Valkey  │          │ Primary   │         │   │
│  │                                └───────────┘          └───────────┘         │   │
│  │    ┌───────────┐                                                            │   │
│  │    │ Memory    │               ┌───────────┐          ┌───────────┐         │   │
│  │    │ 1-2GB     │               │ Memory    │          │ Memory    │         │   │
│  │    │           │               │ 8-16GB    │          │ 32-64GB   │         │   │
│  │    └───────────┘               └───────────┘          └───────────┘         │   │
│  │                                                                             │   │
│  │    ┌───────────┐               ┌───────────┐          ┌───────────┐         │   │
│  │    │ Latency   │               │ Latency   │          │ Latency   │         │   │
│  │    │ p50: 2ms  │               │ p50: 10ms │          │ p50: 50ms │         │   │
│  │    │ p99: 10ms │               │ p99: 50ms │          │ p99: 200ms│         │   │
│  │    └───────────┘               └───────────┘          └───────────┘         │   │
│  │                                                                             │   │
│  │    ┌───────────┐               ┌───────────┐          ┌───────────┐         │   │
│  │    │ RPS       │               │ RPS       │          │ RPS       │         │   │
│  │    │ 5K each   │               │ 50K each  │          │ 100K      │         │   │
│  │    │ 100K total│               │ 150K total│          │           │         │   │
│  │    └───────────┘               └───────────┘          └───────────┘         │   │
│  │                                                                             │   │
│  └─────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                     │
│  Total Capacity: 350K+ RPS with sub-10ms p50 latency globally                      │
│                                                                                     │
└─────────────────────────────────────────────────────────────────────────────────────┘
```

---

*Document Version: 1.0*
*Last Updated: 2025-12-31*
