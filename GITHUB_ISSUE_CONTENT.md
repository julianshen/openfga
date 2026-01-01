# GitHub Issue Content - Copy This to Create RFC Issue

**Instructions:** Copy the content below and create a new GitHub issue at:
`https://github.com/julianshen/openfga/issues/new`

---

# RFC-001: Rust Reimplementation and Edge Architecture for OpenFGA

**Status:** 🟡 Proposed (Awaiting Review)
**RFC Document:** [RFC-001-RUST-EDGE-ARCHITECTURE.md](../blob/claude/rsfga-edge-design-nY8qo/RFC-001-RUST-EDGE-ARCHITECTURE.md)
**PR:** #6
**Authors:** @julianshen
**Stakeholders:** @openfga/maintainers

---

## Quick Summary

This RFC proposes a **phased migration** to a hybrid architecture with **Rust-based edge nodes** for sub-millisecond authorization checks (<1ms P95) while maintaining a **Go-based central cluster** for writes and complex queries. The goal is to enable latency-sensitive use cases (real-time collaboration, gaming, financial services) while maintaining 100% API compatibility.

---

## Motivation

### Current Limitations

**Critical:** Check operation P95 latency is 10-50ms, which is too high for:
- Real-time collaboration (Google Docs, Figma)
- Gaming (multiplayer with fine-grained permissions)
- Financial services (trading platforms)

**Root Causes:**
1. Graph traversal during check (5-50ms)
2. GC pauses in Go (1-10ms)
3. No geographic distribution (single-region adds 50-300ms for global users)
4. Limited horizontal scaling (per-node cache state, single-node resolution)

### Use Cases This Enables

1. **Real-Time Collaboration** - Requires <1ms authorization
2. **Gaming** - Requires <2ms P99, high throughput
3. **Financial Services** - Requires sub-ms latency, compliance
4. **Global SaaS** - Multi-region with data residency

---

## Proposed Solution

### Architecture

```
┌─────────────────────────────────────────────────┐
│          CLIENT APPLICATIONS                    │
└──────────────────┬──────────────────────────────┘
                   │
    ┌──────────────┼──────────────┐
    ▼              ▼              ▼
┌────────┐   ┌────────┐   ┌────────┐
│Edge US │   │Edge EU │   │Edge AP │  ← Rust: <1ms checks
└───┬────┘   └───┬────┘   └───┬────┘
    └────────────┼────────────┘
                 ▼
      ┌──────────────────┐
      │ Central (Go)     │         ← Writes, complex queries
      │ PostgreSQL+Kafka │
      └──────────────────┘
```

### Key Components

1. **Rust Edge Nodes** - Serve Check API with pre-computed results, <1ms latency
2. **Go Central Cluster** - Handle writes, maintain source of truth (existing code)
3. **Pre-Computation Engine** - Transform tuple changes into check results
4. **Kafka/CDC** - Sync changes to edges (<100ms propagation)

### Consistency Model

- **Eventual consistency** with <100ms typical staleness
- **Strong consistency option** available (route to central)
- **Bounded staleness** <1s worst case

---

## Key Trade-offs

| Pros | Cons |
|------|------|
| ✅ <1ms P95 latency (vs 10-50ms) | ❌ Eventual consistency (not strong) |
| ✅ 50x throughput improvement | ❌ Increased complexity (2 codebases) |
| ✅ Global distribution | ❌ Higher operational overhead |
| ✅ 100% API compatible | ❌ Additional infrastructure costs |
| ✅ Incremental migration (low risk) | ❌ Learning curve for Rust |

---

## Investment Required

### Development Cost
- **Timeline:** 24 months (4 phases)
- **Team:** 4-6 engineers
- **Cost:** $1.74M development cost
- **Risk Level:** Medium-High (justified by reward)

### Infrastructure Cost
- **Small deployments (<100K users):** +$600/month ❌ Not cost-effective
- **Large deployments (>100K users):** -$15K/month ✅ 32% cost savings
- **Break-even:** ~100K users or 1M requests/second

### Migration Phases

1. **Phase 0: Foundation** (Month 0-3) - Infrastructure, no production changes
2. **Phase 1: Prototype** (Month 4-6) - Build and benchmark, staging only
3. **Phase 2: Production Pilot** (Month 7-12) - Single region, 5% traffic, opt-in
4. **Phase 3: Global Rollout** (Month 13-18) - Multi-region deployment
5. **Phase 4: Optimization** (Month 19-24) - Performance tuning

Each phase has **rollback capability**.

---

## Questions for Reviewers

### Critical Questions

1. **Is <1ms latency a customer requirement?**
   - Do we have customers asking for this?
   - What's the current customer pain level?

2. **Is Rust the right choice?**
   - Could we achieve similar results with Go optimizations?
   - Do we have Rust expertise on the team?

3. **Is the investment justified?**
   - $1.74M development + infrastructure costs
   - 24-month timeline
   - Will this provide competitive advantage?

4. **What's the risk tolerance?**
   - Medium-high risk (new language, distributed system)
   - Can we afford to fail?

5. **What about eventual consistency?**
   - Will customers accept <100ms staleness?
   - How many use cases require strong consistency?

### Technical Questions

6. **Pre-computation complexity:**
   - Can it scale? (worst case: one change affects millions)
   - What's the fallback if pre-computation falls behind?

7. **Operational complexity:**
   - Can we operate a distributed edge architecture?
   - Do we have SRE capacity?

---

## Open Questions (from RFC)

These must be resolved before Phase 1:

1. **Staleness bound:** <100ms, <500ms, or <1s? (Customer input needed)
2. **Message queue:** Kafka vs NATS vs Pulsar? (Technical evaluation)
3. **Schema migrations:** How to handle with distributed edges? (Design needed)
4. **Fallback strategy:** Serve stale, route to central, or error? (Product decision)
5. **Pricing:** Premium tier, usage-based, or enterprise-only? (Business)
6. **SLAs:** Same (99.9%), higher (99.99%), or best-effort? (Product + SRE)
7. **Ownership:** Dedicated team, rotate, or hybrid? (Org planning)

---

## Decision Needed By

**Target Date:** 2026-03-01 (8 weeks from now)

**Reason:**
- Need to allocate 2026 H1 engineering resources
- Pilot customers waiting for latency improvements
- Competitive pressure from specialized solutions

**If approved:**
- Begin Phase 0 (Foundation) in April 2026
- Target prototype by July 2026
- Production pilot by January 2027

**If deferred:**
- Revisit in Q3 2026
- Focus on Go optimizations instead
- Track customer latency requirements

---

## References

### Documents (in PR #6)

- **RFC:** [RFC-001-RUST-EDGE-ARCHITECTURE.md](../blob/claude/rsfga-edge-design-nY8qo/RFC-001-RUST-EDGE-ARCHITECTURE.md) - Complete proposal
- **Architecture Review:** [ARCHITECTURE_REVIEW.md](../blob/claude/rsfga-edge-design-nY8qo/ARCHITECTURE_REVIEW.md) - Bottleneck analysis
- **Edge Architecture:** [docs/architecture/EDGE_ARCHITECTURE.md](../blob/claude/rsfga-edge-design-nY8qo/docs/architecture/EDGE_ARCHITECTURE.md)
- **Pre-computation:** [docs/architecture/PRECOMPUTATION_ENGINE.md](../blob/claude/rsfga-edge-design-nY8qo/docs/architecture/PRECOMPUTATION_ENGINE.md)
- **Sub-ms Design:** [docs/architecture/SUB_MS_CHECK_DESIGN.md](../blob/claude/rsfga-edge-design-nY8qo/docs/architecture/SUB_MS_CHECK_DESIGN.md)
- **Implementation:** [docs/edge/](../blob/claude/rsfga-edge-design-nY8qo/docs/edge/) (4 documents)

### Checklist

- **Review Checklist:** [PR_REVIEW_CHECKLIST.md](../blob/claude/rsfga-edge-design-nY8qo/PR_REVIEW_CHECKLIST.md) - 61 items to address

### External References

- [Google Zanzibar paper](https://research.google/pubs/pub48190/) - Authorization at scale
- [Rust vs Go performance](https://benchmarksgame-team.pages.debian.net/benchmarksgame/)
- Similar systems: Authzed, Oso, Warrant, AWS Verified Permissions

---

## How to Review

1. **Read the RFC** (30 min): [RFC-001-RUST-EDGE-ARCHITECTURE.md](../blob/claude/rsfga-edge-design-nY8qo/RFC-001-RUST-EDGE-ARCHITECTURE.md)
2. **Review detailed design** (1-2 hours): Documents in PR #6
3. **Consider questions** above
4. **Leave feedback** on this issue or PR #6
5. **Vote** (if maintainer): Approve / Reject / Defer

---

## Next Steps

- [ ] **Community Discussion:** Open GitHub Discussion for broader feedback
- [ ] **RFC Review Meeting:** Schedule with @openfga/maintainers
- [ ] **Design Partners:** Identify 3-5 customers for pilot
- [ ] **Prototype:** Build minimal viable edge (if approved)

---

**Labels:** `rfc`, `proposal`, `breaking-change`, `needs-discussion`, `performance`
**Milestone:** 2026-H1 Planning
