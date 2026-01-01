# RFC-001: Rust Reimplementation and Edge Architecture for OpenFGA

**Status:** 🟡 Proposed (Awaiting Review)
**RFC PR:** #6
**Tracking Issue:** [Create tracking issue]
**Authors:** Architecture Team
**Created:** 2026-01-01
**Last Updated:** 2026-01-01
**Reviewers:** @openfga/maintainers
**Stakeholders:** OpenFGA Core Team, Community, Enterprise Users

---

## Table of Contents

1. [Executive Summary](#executive-summary)
2. [Motivation](#motivation)
3. [Proposed Solution](#proposed-solution)
4. [Detailed Design](#detailed-design)
5. [Alternatives Considered](#alternatives-considered)
6. [Performance Analysis](#performance-analysis)
7. [Migration Strategy](#migration-strategy)
8. [Security & Compliance](#security--compliance)
9. [Cost Analysis](#cost-analysis)
10. [Risks & Mitigations](#risks--mitigations)
11. [Success Metrics](#success-metrics)
12. [Open Questions](#open-questions)
13. [References](#references)
14. [Decision](#decision)

---

## Executive Summary

### The Proposal

We propose a **phased migration** of OpenFGA to a hybrid architecture consisting of:
1. **Rust-based edge nodes** for ultra-low latency authorization checks (<1ms P95)
2. **Central Go cluster** for write operations and complex queries (maintaining compatibility)
3. **Pre-computation engine** to materialize authorization results at edges
4. **Multi-region deployment** with product-based partitioning

### Key Goals

| Goal | Current | Target | Approach |
|------|---------|--------|----------|
| Check Latency P95 | 10-50ms | <1ms | Edge + Pre-computation |
| Check Latency P99 | 50-100ms | <2ms | Rust, no GC |
| Throughput | 10K checks/s | 500K checks/s | Distributed edge nodes |
| Global Availability | 99.9% | 99.99% | Multi-region redundancy |

### Investment Required

- **Development:** 18-24 months, 4-6 engineers
- **Infrastructure:** Additional edge nodes (cost analysis in Section 9)
- **Risk:** Medium-High (new language, distributed architecture)

### Why This Matters

OpenFGA is increasingly used for **latency-sensitive** applications (real-time collaboration, gaming, financial services). Current architecture cannot achieve sub-millisecond latency required by these use cases. This proposal addresses this gap while maintaining backward compatibility.

---

## Motivation

### Current Limitations

#### 1. Latency Bottlenecks (Critical)

**Problem:** Current P95 latency of 10-50ms is too high for real-time applications.

**Root Causes:**
- Graph traversal during check (5-50ms)
- GC pauses in Go (1-10ms)
- Database round-trips (1-5ms)
- Single-node resolution (no parallelization)

**Impact:**
- Cannot support real-time collaboration (Google Docs, Figma)
- Poor user experience for interactive applications
- Losing customers to specialized solutions (Oso, Warrant)

#### 2. Horizontal Scaling Limitations (High)

**Problem:** Cannot efficiently scale check operations beyond single-node performance.

**Root Causes:**
- Per-node cache state (low hit rates in clusters)
- No distributed graph resolution
- All nodes share same database (bottleneck)

**Impact:**
- Vertical scaling required (expensive)
- Poor resource utilization
- Difficulty serving global traffic

#### 3. No Geographic Distribution (Medium)

**Problem:** Single-region deployment adds latency for global users.

**Root Causes:**
- No edge deployment model
- No data partitioning strategy
- Cross-region latency (50-300ms)

**Impact:**
- Poor experience for non-US users
- Cannot meet data residency requirements
- Compliance issues (GDPR, etc.)

### Use Cases This Enables

1. **Real-Time Collaboration**
   - Google Docs-like applications
   - Figma-like design tools
   - Requires: <1ms authorization checks

2. **Gaming**
   - Multiplayer games with fine-grained permissions
   - Requires: <2ms P99, high throughput

3. **Financial Services**
   - Real-time trading platforms
   - Requires: Sub-millisecond latency, compliance

4. **Global SaaS**
   - Multi-region deployment
   - Requires: Data residency, low latency globally

### Why Now?

- **Market demand:** Multiple customers requesting <5ms latency
- **Competition:** Specialized solutions emerging (Oso, Authzed edge)
- **Technology maturity:** Rust ecosystem stable, edge computing proven
- **OpenFGA adoption:** Growing user base ready to fund development

---

## Proposed Solution

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        CLIENT APPLICATIONS                      │
└────────────────────────────┬────────────────────────────────────┘
                             │
              ┌──────────────┼──────────────┐
              │              │              │
              ▼              ▼              ▼
    ┌─────────────┐  ┌─────────────┐  ┌─────────────┐
    │   Edge US   │  │  Edge EU    │  │ Edge APAC   │
    │   (Rust)    │  │  (Rust)     │  │  (Rust)     │
    │ • Check <1ms│  │ • Check <1ms│  │ • Check <1ms│
    │ • Read-only │  │ • Read-only │  │ • Read-only │
    └──────┬──────┘  └──────┬──────┘  └──────┬──────┘
           │                │                │
           └────────────────┼────────────────┘
                            │
                            ▼
              ┌───────────────────────────┐
              │   Central Cluster (Go)    │
              │ • Write operations        │
              │ • Complex queries         │
              │ • Full OpenFGA API        │
              └────────────┬──────────────┘
                           │
                           ▼
              ┌───────────────────────────┐
              │   PostgreSQL + Kafka      │
              │ • Source of truth         │
              │ • Change data capture     │
              └───────────────────────────┘
```

### Key Components

#### 1. Rust Edge Nodes (New)

**Responsibility:** Serve Check API with <1ms latency

**Technology Stack:**
- Language: Rust (stable)
- gRPC: tonic + prost
- Storage: DashMap (lock-free HashMap)
- Deployment: Kubernetes sidecar

**Data Model:**
```rust
// Pre-computed check results, not tuples
pub struct EdgeState {
    // Hash(store, object, relation, user) -> allowed
    checks: DashMap<u64, CheckResult>,

    // Authorization models (small, ~10-100KB)
    models: DashMap<String, AuthModel>,
}
```

#### 2. Pre-Computation Engine (New)

**Responsibility:** Transform tuple changes into check results

**Process:**
1. Listen to tuple changes (Kafka CDC)
2. Determine affected (object, relation, user) combinations
3. Compute check results using full graph resolution
4. Publish to edge nodes

**Complexity:**
- Best case: O(1) - direct assignment
- Worst case: O(N×M) - group membership affecting all objects
- Mitigation: Rate limiting, priority queues

#### 3. Central Go Cluster (Existing + Enhanced)

**Responsibility:** Write path, complex queries, source of truth

**Changes:**
- Add CDC publishing to Kafka
- Add pre-computation trigger
- Maintain full OpenFGA API compatibility

#### 4. Sync Infrastructure (New)

**Responsibility:** Propagate changes to edges

**Technology:**
- Message Queue: Kafka or NATS
- Protocol: Protobuf over Kafka
- Latency: <100ms propagation time

---

## Detailed Design

> **Note:** Detailed design documents are in:
> - `/docs/architecture/EDGE_ARCHITECTURE.md` - Edge node design
> - `/docs/architecture/PRECOMPUTATION_ENGINE.md` - Pre-computation algorithm
> - `/docs/architecture/SUB_MS_CHECK_DESIGN.md` - Sub-millisecond check design
> - `/docs/edge/01_DESIGN_DOCUMENT.md` - Implementation details

### Request Flow

#### Check Request (Read Path)

```
1. Client → Edge Node (gRPC, <0.1ms network)
2. Edge: HashMap lookup (O(1), <0.1ms)
3. Edge → Client (return result)

Total: <0.5ms P95, <1ms P99
```

#### Write Request (Write Path)

```
1. Client → Central Cluster (existing path)
2. Central: Validate and write to PostgreSQL
3. Central: Publish CDC event to Kafka
4. Pre-Compute Worker: Compute affected checks
5. Kafka → Edge Nodes (async, <100ms)
6. Edge: Update in-memory cache

Total for write: Same as current (~10-50ms)
Propagation delay: <100ms
```

### Data Consistency Model

**Consistency Guarantee:** Eventual consistency with bounded staleness

- **Write-Read Consistency:** NOT guaranteed (edge may be stale)
- **Staleness Bound:** <100ms typical, <1s worst case
- **Strong Consistency Option:** Route to central cluster (existing behavior)

**Trade-off:**
- ✅ Ultra-low latency for reads
- ❌ Temporary inconsistency after writes
- ✅ Opt-in strong consistency for critical operations

### Storage Requirements

**Per Edge Node:**

| Scale | Users | Objects | Check Results | Memory |
|-------|-------|---------|---------------|--------|
| Small | 1K | 50K | 150K | ~7 MB |
| Medium | 10K | 500K | 5M | ~240 MB |
| Large | 100K | 2M | 100M | ~4.8 GB |

**Partitioning Strategy:** Product-based (each edge stores subset of products)

---

## Alternatives Considered

### Alternative 1: Optimize Go Implementation

**Approach:** Improve current Go codebase with caching, singleflight, etc.

**Pros:**
- Lower risk (same language)
- Faster to implement
- No migration needed

**Cons:**
- ❌ Cannot eliminate GC pauses (still 1-10ms)
- ❌ Fundamental limits to single-node performance
- ❌ Does not address geographic distribution

**Decision:** Rejected - Cannot achieve <1ms target with GC

### Alternative 2: Full Rust Rewrite

**Approach:** Rewrite entire OpenFGA in Rust immediately

**Pros:**
- ✅ Clean slate, optimal design
- ✅ Maximum performance

**Cons:**
- ❌ Very high risk (18-36 months)
- ❌ Disrupts existing users
- ❌ Expensive development

**Decision:** Rejected - Too risky, prefer incremental approach

### Alternative 3: Use Existing Edge CDN (Cloudflare Workers, Lambda@Edge)

**Approach:** Deploy authorization logic to existing edge platforms

**Pros:**
- ✅ Fast deployment
- ✅ Global distribution included

**Cons:**
- ❌ Limited memory (128MB-256MB on Cloudflare)
- ❌ Cold start latency
- ❌ Vendor lock-in
- ❌ Cost at scale (very expensive)

**Decision:** Rejected - Too expensive, insufficient memory

### Alternative 4: Hybrid Approach (Proposed)

**Approach:** Rust edges for reads, Go central for writes

**Pros:**
- ✅ Achieves latency target
- ✅ Maintains compatibility
- ✅ Incremental migration
- ✅ Lower risk than full rewrite

**Cons:**
- ⚠️ Increased complexity
- ⚠️ Need to maintain two codebases
- ⚠️ Eventual consistency model

**Decision:** ✅ **Selected** - Best balance of risk and reward

---

## Performance Analysis

### Projected Performance

> **⚠️ Important:** These are **estimated targets** based on preliminary analysis. Actual performance requires validation through prototyping and benchmarking.

#### Latency Improvements

| Operation | Current (Go) | Target (Rust Edge) | Improvement |
|-----------|--------------|-------------------|-------------|
| Check P50 | 5-10ms | <0.3ms | **20-30x** |
| Check P95 | 10-50ms | <1ms | **10-50x** |
| Check P99 | 50-100ms | <2ms | **25-50x** |
| BatchCheck P95 | 100-200ms | <5ms | **20-40x** |

#### Throughput Improvements

| Metric | Current | Target | Improvement |
|--------|---------|--------|-------------|
| Checks/sec (single node) | ~10K | ~500K | **50x** |
| Checks/sec (cluster) | ~50K | ~10M | **200x** |

### Validation Plan

**Phase 1: Prototype** (Month 1-2)
- [ ] Build minimal Rust edge prototype
- [ ] Benchmark against Go implementation
- [ ] Validate <1ms target is achievable

**Phase 2: Load Testing** (Month 3-4)
- [ ] Deploy to staging environment
- [ ] Run production-like workloads
- [ ] Measure P50/P95/P99 under load

**Phase 3: Production Pilot** (Month 5-6)
- [ ] Deploy to single customer (opt-in)
- [ ] Monitor real-world performance
- [ ] Iterate based on feedback

**Success Criteria:**
- ✅ P95 latency <1ms in production
- ✅ P99 latency <2ms in production
- ✅ No increase in error rate
- ✅ Customer satisfaction score >4.5/5

### Assumptions

These performance targets assume:
1. Edge nodes have sufficient memory (8GB+ RAM)
2. Network latency <0.1ms (localhost deployment)
3. Pre-computation keeps up with write rate
4. Authorization models are not pathologically deep (depth <10)
5. Working set fits in edge memory (95%+ hit rate)

If any assumption is violated, performance may degrade.

---

## Migration Strategy

### Overview

**Timeline:** 18-24 months
**Approach:** Incremental, backward-compatible migration
**Phases:** 4 phases with rollback capability at each

### Phase 0: Foundation (Month 0-3)

**Objective:** Build infrastructure without changing production

**Deliverables:**
- [ ] Set up Rust development environment
- [ ] Create CI/CD for Rust components
- [ ] Set up Kafka/CDC infrastructure
- [ ] Build monitoring and observability

**Rollback:** N/A (no production changes)

### Phase 1: Rust Edge Prototype (Month 4-6)

**Objective:** Prove technical feasibility

**Deliverables:**
- [ ] Minimal Rust edge implementation
- [ ] Pre-computation engine prototype
- [ ] Kafka sync mechanism
- [ ] Benchmark suite

**Deployment:** Staging only, no production traffic

**Rollback:** Simply turn off prototype

### Phase 2: Production Pilot (Month 7-12)

**Objective:** Validate in production with limited scope

**Deliverables:**
- [ ] Production-ready Rust edge
- [ ] Deploy to single region (US-East)
- [ ] Opt-in for select customers
- [ ] Comprehensive monitoring

**Deployment Strategy:**
```
┌─────────────────────────────────────┐
│ Traffic Split:                      │
│ • 95% → Go central (existing)       │
│ • 5% → Rust edge (pilot)            │
│                                     │
│ Customers:                          │
│ • Opt-in only                       │
│ • Can revert instantly              │
└─────────────────────────────────────┘
```

**Rollback:** Route pilot customers back to Go central

### Phase 3: Global Rollout (Month 13-18)

**Objective:** Deploy globally with product partitioning

**Deliverables:**
- [ ] Multi-region edge deployment
- [ ] Product-based partitioning
- [ ] Automated scaling
- [ ] Disaster recovery procedures

**Deployment Strategy:**
```
Region-by-region rollout:
1. US-East (pilot already running)
2. US-West (week 1-2)
3. EU-West (week 3-4)
4. APAC (week 5-6)

Traffic migration per region:
• Week 1: 10% traffic
• Week 2: 30% traffic
• Week 3: 60% traffic
• Week 4: 100% traffic
```

**Rollback:** Revert specific regions to central

### Phase 4: Optimization (Month 19-24)

**Objective:** Optimize costs and performance

**Deliverables:**
- [ ] Cost optimization (right-sizing edges)
- [ ] Performance tuning
- [ ] Advanced features (conditional checks, etc.)
- [ ] Decommission redundant Go code (if applicable)

**Rollback:** N/A (optimizations are incremental)

### Backward Compatibility

**API Compatibility:** 100% - Rust edge implements same gRPC/HTTP API

**Client Libraries:** No changes required

**Data Format:** Compatible with existing PostgreSQL schema

**Deprecation Policy:** No existing features deprecated

---

## Security & Compliance

### Security Architecture

#### Edge-to-Central Authentication

**Mechanism:** Mutual TLS (mTLS) with certificate rotation

```
Edge Node ←mTLS→ Central Cluster
• Edge cert issued by central CA
• 30-day rotation
• Certificate pinning
```

#### Data Encryption

| Layer | Encryption | Key Management |
|-------|------------|----------------|
| In Transit | TLS 1.3 | Let's Encrypt + custom CA |
| At Rest (Central) | PostgreSQL encryption | Cloud KMS |
| At Rest (Edge) | Memory only (no disk) | N/A |
| Kafka | TLS + SASL | Kafka ACLs |

#### Audit Logging

**Distributed Audit Architecture:**
```
Edge Node → Local buffer (10s)
         → Kafka (audit topic)
         → S3 (long-term storage)
         → SIEM (real-time alerts)
```

**Retention:** 90 days hot, 7 years cold (compliance)

### Compliance Considerations

#### Data Residency (GDPR, etc.)

**Solution:** Product-based partitioning ensures data stays in region

```
EU Product → EU Edge Nodes only
US Product → US Edge Nodes only
Global Product → All regions (with consent)
```

**Controls:**
- [ ] Configurable per product
- [ ] Audit trail of data location
- [ ] Ability to restrict replication

#### SOC 2 / ISO 27001

**Impact:** New controls needed for edge architecture

**Additional Controls:**
- [ ] Edge node access logging
- [ ] Immutable audit logs
- [ ] Incident response for edge compromise
- [ ] Penetration testing of edge nodes

#### Privacy (PII Handling)

**Design:** Edges store pre-computed results, not raw tuples

**Benefit:** Reduces PII exposure (only boolean results, not relationships)

**Consideration:** Models may contain sensitive data (review needed)

---

## Cost Analysis

### Development Costs

| Phase | Duration | Team Size | Cost (loaded) |
|-------|----------|-----------|---------------|
| Foundation | 3 months | 2 engineers | $120K |
| Prototype | 3 months | 3 engineers | $180K |
| Pilot | 6 months | 4 engineers | $480K |
| Rollout | 6 months | 5 engineers | $600K |
| Optimization | 6 months | 3 engineers | $360K |
| **Total** | **24 months** | **4-5 avg** | **$1.74M** |

### Infrastructure Costs (Monthly)

#### Small Deployment (1K users, 3 regions)

| Component | Qty | Unit Cost | Monthly |
|-----------|-----|-----------|---------|
| Edge nodes (t3.medium) | 9 | $30 | $270 |
| Central Go cluster | 3 | $100 | $300 |
| PostgreSQL | 1 | $200 | $200 |
| Kafka | 3 brokers | $100 | $300 |
| Bandwidth | 500GB | $0.08/GB | $40 |
| **Total** | | | **$1,110** |

**Current cost:** $500/month
**Delta:** +$610/month ❌ (not cost-effective at small scale)

#### Large Deployment (1M users, 12 regions)

| Component | Qty | Unit Cost | Monthly |
|-----------|-----|-----------|---------|
| Edge nodes (r5.2xlarge) | 48 | $300 | $14,400 |
| Central Go cluster | 10 | $500 | $5,000 |
| PostgreSQL (RDS) | 1 primary + 3 replicas | $2,000 | $8,000 |
| Kafka cluster | 12 brokers | $200 | $2,400 |
| Bandwidth | 10TB | $0.05/GB | $500 |
| **Total** | | | **$30,300** |

**Current cost:** $45,000/month (estimated)
**Delta:** -$14,700/month ✅ (32% cost savings)

### Break-Even Analysis

**Break-even point:** ~100K users or 1M requests/second

**Below break-even:** Edge architecture costs more (not recommended)

**Above break-even:** Edge architecture saves money + provides better performance

### Cost Optimization Strategies

1. **Auto-scaling:** Scale edge nodes based on traffic
2. **Spot instances:** Use spot for non-critical edges (40% savings)
3. **Product partitioning:** Only deploy edges for high-traffic products
4. **Tiered offerings:** Offer edge as premium tier

---

## Risks & Mitigations

### Technical Risks

#### R1: Rust Learning Curve (Medium Risk, High Impact)

**Risk:** Team unfamiliar with Rust, slower development

**Probability:** 70%
**Impact:** +3-6 months timeline

**Mitigation:**
- [ ] Hire 1-2 experienced Rust engineers
- [ ] 2-month Rust training for team
- [ ] Start with small, isolated components
- [ ] Code reviews from Rust experts

#### R2: Pre-Computation Complexity (High Risk, High Impact)

**Risk:** Pre-computation doesn't scale, edge data too large

**Probability:** 40%
**Impact:** Feature doesn't work, wasted investment

**Mitigation:**
- [ ] Build prototype early (Month 1-2)
- [ ] Test with real production models
- [ ] Have fallback: Hot-path materialization (only pre-compute top 95%)
- [ ] Circuit breaker: Route to central if edge too large

#### R3: Eventual Consistency Issues (Medium Risk, Medium Impact)

**Risk:** Customers expect strong consistency, complain about stale data

**Probability:** 50%
**Impact:** Customer dissatisfaction, support burden

**Mitigation:**
- [ ] Clear documentation of consistency model
- [ ] Opt-in only (customers choose edge)
- [ ] Provide "strong consistency" option (route to central)
- [ ] SLO on staleness (<100ms typical)

#### R4: Operational Complexity (High Risk, Medium Impact)

**Risk:** Edge architecture harder to operate, more incidents

**Probability:** 60%
**Impact:** Increased on-call burden, reliability issues

**Mitigation:**
- [ ] Invest heavily in monitoring and observability
- [ ] Automated canary deployments
- [ ] Comprehensive runbooks
- [ ] Dedicated SRE for edge architecture

### Business Risks

#### B1: Customer Adoption (Medium Risk, High Impact)

**Risk:** Customers don't opt-in to edge, no ROI

**Probability:** 30%
**Impact:** Wasted $1.74M investment

**Mitigation:**
- [ ] Early customer engagement (design partners)
- [ ] Pilot with 3-5 enthusiastic customers
- [ ] Clear value proposition (<1ms latency)
- [ ] Easy rollback for hesitant customers

#### B2: Competition (Low Risk, Medium Impact)

**Risk:** Competitors release similar feature first

**Probability:** 20%
**Impact:** Reduced competitive advantage

**Mitigation:**
- [ ] Monitor competitor releases
- [ ] Move quickly through phases
- [ ] Focus on OpenFGA-specific advantages (open source, flexibility)

### Mitigation Summary

**Overall Risk Level:** Medium-High (justified by potential reward)

**Go/No-Go Criteria:**
- ✅ Prototype achieves <1ms P95 (Month 2)
- ✅ 3+ customers commit to pilot (Month 6)
- ✅ Production pilot successful (Month 12)

If any criterion fails, **halt and reassess**.

---

## Success Metrics

### Technical Metrics

| Metric | Baseline (Go) | Target (Rust Edge) | Timeline |
|--------|---------------|-------------------|----------|
| Check P95 latency | 10-50ms | <1ms | Month 12 |
| Check P99 latency | 50-100ms | <2ms | Month 12 |
| Throughput/node | 10K checks/s | 500K checks/s | Month 12 |
| Cache hit rate | 80% | 95%+ | Month 18 |
| Availability | 99.9% | 99.99% | Month 24 |

### Business Metrics

| Metric | Baseline | Target | Timeline |
|--------|----------|--------|----------|
| Customer satisfaction | 4.2/5 | 4.7/5 | Month 18 |
| Enterprise customers using edge | 0 | 10+ | Month 24 |
| Latency-sensitive use cases | 5% | 30% | Month 24 |
| Infrastructure cost/customer | $X | 0.7X | Month 24 |

### Adoption Metrics

| Metric | Target | Timeline |
|--------|--------|----------|
| Customers in pilot | 3-5 | Month 6 |
| Customers in production | 20+ | Month 18 |
| % of Check traffic on edge | 50%+ | Month 24 |

### Monitoring & Reporting

**Weekly:** Engineering metrics (latency, throughput, errors)
**Monthly:** Business metrics (adoption, satisfaction, cost)
**Quarterly:** Executive review with go/no-go decision

---

## Open Questions

> These questions must be resolved before implementation begins.

### Technical Questions

1. **Q:** What's the acceptable staleness bound for edge data?
   - **Options:** A) <100ms, B) <500ms, C) <1s, D) Configurable per product
   - **Status:** ⚠️ Unresolved - needs customer input
   - **Owner:** Product team
   - **Deadline:** Month 1

2. **Q:** Which message queue: Kafka vs. NATS vs. Pulsar?
   - **Options:** A) Kafka (mature, heavy), B) NATS (fast, lighter), C) Pulsar (hybrid)
   - **Status:** ⚠️ Unresolved - needs technical evaluation
   - **Owner:** Architecture team
   - **Deadline:** Month 2

3. **Q:** How to handle schema migrations with edge nodes?
   - **Options:** A) Central push, B) Edge pull, C) Blue-green edge deployment
   - **Status:** ⚠️ Unresolved - needs design
   - **Owner:** Platform team
   - **Deadline:** Month 3

4. **Q:** What's the fallback strategy when edge is out of sync?
   - **Options:** A) Serve stale, B) Route to central, C) Return error
   - **Status:** ⚠️ Unresolved - needs product decision
   - **Owner:** Product team
   - **Deadline:** Month 2

### Business Questions

5. **Q:** How to price edge deployment?
   - **Options:** A) Premium tier (+50%), B) Usage-based, C) Enterprise-only
   - **Status:** ⚠️ Unresolved - needs pricing analysis
   - **Owner:** Business team
   - **Deadline:** Month 6 (before pilot)

6. **Q:** What SLAs to offer for edge deployment?
   - **Options:** A) Same as central (99.9%), B) Higher (99.99%), C) Lower (best-effort)
   - **Status:** ⚠️ Unresolved - needs cost/benefit analysis
   - **Owner:** Product + SRE
   - **Deadline:** Month 6

### Organizational Questions

7. **Q:** Who owns Rust codebase long-term?
   - **Options:** A) Dedicated team, B) Rotate ownership, C) Hybrid
   - **Status:** ⚠️ Unresolved - needs org planning
   - **Owner:** Engineering leadership
   - **Deadline:** Month 3

---

## References

### Research & Benchmarks

1. [Go vs. Rust Performance Comparison](https://benchmarksgame-team.pages.debian.net/benchmarksgame/)
2. DashMap benchmarks: [GitHub - xacrimon/dashmap](https://github.com/xacrimon/dashmap)
3. Edge computing patterns: [Cloudflare Workers](https://workers.cloudflare.com/), [AWS Lambda@Edge](https://aws.amazon.com/lambda/edge/)
4. Authorization at scale: [Google Zanzibar paper](https://research.google/pubs/pub48190/)

### Similar Systems

1. **Authzed SpiceDB:** PostgreSQL + in-memory caching (Go)
2. **Oso:** Policy engine with caching (Python/Rust)
3. **Warrant:** Edge authorization (Go)
4. **AWS Verified Permissions:** Cedar policy engine (Rust)

### OpenFGA References

1. Current architecture: `docs/architecture/architecture.md`
2. Bottleneck analysis: `ARCHITECTURE_REVIEW.md` (this PR)
3. Storage backends: `pkg/storage/`
4. Check algorithm: `internal/graph/`

---

## Decision

> **To be completed after RFC review process**

### Status

- [ ] **Proposed** (current)
- [ ] **Under Review** (discussion opened)
- [ ] **Accepted** (approved by maintainers)
- [ ] **Rejected** (not moving forward)
- [ ] **Deferred** (revisit later)

### Decision Date

TBD (after review)

### Decision Makers

- [ ] @maintainer-1 (approve/reject)
- [ ] @maintainer-2 (approve/reject)
- [ ] @maintainer-3 (approve/reject)

### Decision Outcome

> To be filled after decision:
>
> **Decision:** [Accepted / Rejected / Deferred]
>
> **Reasoning:** [Why this decision was made]
>
> **Conditions:** [Any conditions for acceptance]
>
> **Next Steps:** [What happens next]

### Approval Signatures

```
Approved by:
- [ ] [Name], [Role], [Date]
- [ ] [Name], [Role], [Date]
- [ ] [Name], [Role], [Date]
```

---

## Appendix: Document Index

This RFC references the following detailed design documents:

1. **ARCHITECTURE_REVIEW.md** - Bottleneck analysis of current system
2. **docs/architecture/EDGE_ARCHITECTURE.md** - Edge node architecture details
3. **docs/architecture/PRECOMPUTATION_ENGINE.md** - Pre-computation algorithm
4. **docs/architecture/SUB_MS_CHECK_DESIGN.md** - Sub-millisecond check design
5. **docs/edge/01_DESIGN_DOCUMENT.md** - Rust implementation design
6. **docs/edge/02_SPEC_DOCUMENT.md** - API and data specifications
7. **docs/edge/03_TEST_DOCUMENT.md** - Testing strategy
8. **docs/edge/04_REFERENCE_DOCUMENT.md** - Code examples and references

**Reading Order:**
1. Start with this RFC for overview
2. Read ARCHITECTURE_REVIEW.md for problem statement
3. Read EDGE_ARCHITECTURE.md for solution architecture
4. Read design documents (01-04) for implementation details

---

**RFC Shepherd:** [Assign an RFC shepherd]
**Last Updated:** 2026-01-01
**Next Review:** TBD (schedule RFC review meeting)
