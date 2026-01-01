# Blocker B2: Performance Claims - Required Changes

This document lists all performance claims that need qualifiers and assumptions.

---

## Summary of Changes Needed

**Total Performance Claims Found:** 15+
**Documents Affected:** 6 files
**Changes Required:** Add qualifiers + assumptions section

---

## Changes by Document

### 1. ARCHITECTURE_REVIEW.md

#### Line 11: Performance Improvement Claim

**CURRENT:**
```markdown
- A Rust reimplementation could yield 2-5x performance improvements with proper design
```

**SHOULD BE:**
```markdown
- A Rust reimplementation could **potentially** yield 2-5x performance improvements with proper design **based on preliminary analysis** (see Performance Assumptions section)
```

#### Add New Section After Executive Summary

**INSERT AFTER LINE 13:**

```markdown
---

## ⚠️ Performance Assumptions & Disclaimers

**IMPORTANT:** All performance projections in this document are **estimates based on preliminary analysis**. They have NOT been validated through prototyping or benchmarking.

### Assumptions

These performance targets assume:

1. **Hardware:** Edge nodes with 8GB+ RAM, modern CPUs (2020+)
2. **Network:** <0.1ms latency between application and edge (localhost deployment)
3. **Data Characteristics:**
   - Authorization models are not pathologically deep (depth <10)
   - Working set fits in edge memory (95%+ cache hit rate)
   - Pre-computation keeps up with write rate
4. **Workload:** Read-heavy (90%+ reads vs writes)
5. **Implementation:** Optimal Rust code following best practices

### Validation Plan

**Phase 1 (Month 1-2):** Build prototype and benchmark
- [ ] Validate 2-5x improvement claim
- [ ] Measure actual P95/P99 latencies
- [ ] Test with production-like data

**Phase 2 (Month 3-4):** Load testing
- [ ] Test under concurrent load
- [ ] Measure GC pause elimination
- [ ] Validate throughput claims

**Success Criteria:**
- ✅ P95 <1ms achieved in prototype
- ✅ Throughput >100K checks/s per node
- ✅ No degradation under load

**If validation fails:** Reassess approach or abandon proposal

### Comparable Systems

Performance claims are informed by:
- **Rust vs Go benchmarks:** https://benchmarksgame-team.pages.debian.net/benchmarksgame/
- **DashMap benchmarks:** Concurrent HashMap performance data
- **Similar systems:** AWS Verified Permissions (Rust), Authzed (Go)

---
```

### 2. docs/architecture/SUB_MS_CHECK_DESIGN.md

#### Line 5: Latency Target

**CURRENT:**
```markdown
**Goal**: Achieve <1ms P95 latency for Check operations at edge nodes.
```

**SHOULD BE:**
```markdown
**Goal**: Achieve **target of <1ms P95 latency** for Check operations at edge nodes.

**Status:** ⚠️ **Unvalidated Target** - Requires prototyping to confirm achievability
```

#### Line 18-90: Latency Budget Table

**ADD HEADER BEFORE TABLE:**
```markdown
### 2.1 Latency Budget Breakdown (Estimated)

> **⚠️ IMPORTANT:** These are **projected estimates** based on analysis of Rust performance characteristics and similar systems. Actual measurements required in Phase 1 prototype.
```

**UPDATE TABLE HEADER:**
```markdown
│  Component                          │ Current (Go) │ **Target** (Rust) │ Technique     │
```

### 3. docs/edge/01_DESIGN_DOCUMENT.md

#### Line 20: Throughput Claim

**CURRENT:**
```markdown
| Throughput per edge | 500K checks/s | Rust implementation, lock-free |
```

**SHOULD BE:**
```markdown
| Throughput per edge | **Target: 500K checks/s** | Rust implementation, lock-free **(requires validation)** |
```

#### Line 22: API Compatibility

**CURRENT:**
```markdown
| API compatibility | 100% | Same gRPC/HTTP API as OpenFGA |
```

**SHOULD BE:**
```markdown
| API compatibility | **Goal: 100%** | Same gRPC/HTTP API as OpenFGA |
```

### 4. RFC-001-RUST-EDGE-ARCHITECTURE.md

#### Section: Performance Analysis

**ADD WARNING AT TOP OF PERFORMANCE SECTION:**

```markdown
## Performance Analysis

> ### ⚠️ Performance Projections Disclaimer
>
> **All performance numbers below are ESTIMATED TARGETS based on:**
> - Preliminary analysis of current bottlenecks
> - Rust vs Go benchmark comparisons from public sources
> - Performance characteristics of proposed data structures (DashMap, etc.)
> - Architecture patterns from similar systems
>
> **These numbers have NOT been validated through:**
> - Prototyping with actual OpenFGA workloads
> - Load testing at scale
> - Production deployment
>
> **Validation Plan:**
> - Phase 1 (Month 1-2): Build prototype and benchmark
> - Phase 2 (Month 3-4): Load testing in staging
> - Phase 3 (Month 5-6): Production pilot with monitoring
>
> **If actual performance falls short of targets:**
> - Reassess feasibility
> - Adjust scope (e.g., edge-only for hot data)
> - Consider alternative approaches
> - Abandon if <50% of target performance achieved

### Projected Performance

The table below shows **target metrics** we aim to achieve:
```

#### Update All Performance Tables

**Before each performance table, add:**
```markdown
> **Note:** Targets shown below, not guarantees. See validation plan.
```

### 5. docs/architecture/EDGE_ARCHITECTURE.md

#### Memory Requirements Section

**CURRENT:** Shows specific memory numbers

**ADD BEFORE MEMORY TABLE:**
```markdown
### Memory Requirements (Estimated)

> **⚠️ Estimation Basis:**
> - 48 bytes per pre-computed check result (hash key + value + overhead)
> - Assumes 30% of all possible checks pre-computed (hot data)
> - Based on DashMap memory footprint benchmarks
> - Actual memory usage may vary ±50%

**Validation needed:** Test with production data in Phase 1 prototype
```

### 6. docs/architecture/PRECOMPUTATION_ENGINE.md

#### Complexity Claims

**ADD SECTION:**
```markdown
## Performance Characteristics (Preliminary Analysis)

> **⚠️ Important:** Complexity analysis below is theoretical. Real-world performance depends on:
> - Authorization model structure
> - Data distribution (users per object, objects per user)
> - Write patterns (bursty vs steady)
> - Hardware characteristics

### Change Type Complexity

| Change Type | Theoretical | Real-World Impact |
|-------------|-------------|-------------------|
| Direct assignment | O(1) | Minimal (validated) |
| Computed userset | O(relations) | Low-Medium **(needs validation)** |
| TTU | O(children) | Medium-High **(needs validation)** |
| Group membership | O(N×M) | **High - May require rate limiting** **(needs validation)** |

**Mitigation strategies** for complex cases documented in Section 5.
```

---

## New Section to Add to All Documents

Add this disclaimer at the top of each architecture document:

```markdown
---
> ### 📊 Performance Claims Disclaimer
>
> This document contains **performance projections and targets** that are:
> - ✅ Based on preliminary analysis and research
> - ✅ Informed by similar systems and benchmarks
> - ❌ NOT validated through prototyping
> - ❌ NOT guaranteed to be achievable
>
> **All performance claims require validation** through the phased prototyping and testing approach described in the RFC.
>
> See [RFC-001 Performance Assumptions](#performance-assumptions--disclaimers) for full details.
---
```

---

## Specific Word Replacements Needed

### Global Search & Replace

Run these replacements across all documents:

1. **"will achieve"** → **"targets"** or **"aims to achieve"**
   ```bash
   find . -name "*.md" -exec sed -i 's/will achieve/targets/g' {} \;
   ```

2. **"can achieve"** → **"may be able to achieve"**
   ```bash
   find . -name "*.md" -exec sed -i 's/can achieve/may be able to achieve/g' {} \;
   ```

3. **Unqualified latency numbers** → Add "Target:" prefix
   ```bash
   # Examples:
   # "<1ms P95" → "Target: <1ms P95"
   # "500K checks/s" → "Target: 500K checks/s"
   ```

4. **"X times faster"** → **"potentially X times faster"**
   ```bash
   find . -name "*.md" -exec sed -i 's/\([0-9]\+x\) faster/potentially \1 faster/g' {} \;
   ```

---

## Checklist for Applying Changes

- [ ] 1. Add Performance Assumptions section to ARCHITECTURE_REVIEW.md
- [ ] 2. Add disclaimer to SUB_MS_CHECK_DESIGN.md
- [ ] 3. Update throughput claims in 01_DESIGN_DOCUMENT.md
- [ ] 4. Add performance disclaimer to RFC-001
- [ ] 5. Add estimation basis to EDGE_ARCHITECTURE.md memory table
- [ ] 6. Add complexity disclaimer to PRECOMPUTATION_ENGINE.md
- [ ] 7. Run global search & replace for qualifying language
- [ ] 8. Add disclaimer box to top of each architecture document
- [ ] 9. Review all changed files for readability
- [ ] 10. Ensure all performance numbers have context

---

## Automated Fix Script

```bash
#!/bin/bash
# apply-performance-qualifiers.sh

echo "Applying performance claim qualifiers..."

# Add disclaimer to top of each doc (after title)
for file in docs/architecture/*.md docs/edge/*.md ARCHITECTURE_REVIEW.md; do
    if [ -f "$file" ]; then
        echo "Processing $file..."
        # Insert disclaimer after first heading
        sed -i '2i\\n---\n> ### 📊 Performance Claims Disclaimer\n>\n> This document contains **performance projections and targets** that are:\n> - ✅ Based on preliminary analysis and research\n> - ❌ NOT validated through prototyping\n> - ❌ NOT guaranteed to be achievable\n>\n> **All performance claims require validation** through phased testing.\n---\n' "$file"
    fi
done

# Qualify specific claims
sed -i 's/could yield 2-5x/could **potentially** yield 2-5x/g' ARCHITECTURE_REVIEW.md
sed -i 's/Achieve <1ms/Achieve **target of <1ms**/g' docs/architecture/SUB_MS_CHECK_DESIGN.md
sed -i 's/500K checks\/s/Target: 500K checks\/s/g' docs/edge/01_DESIGN_DOCUMENT.md

echo "Done! Review changes with: git diff"
```

**To apply:**
```bash
chmod +x apply-performance-qualifiers.sh
./apply-performance-qualifiers.sh
git diff  # Review changes
```

---

## After Applying Changes

1. **Review all changes** with `git diff`
2. **Read through documents** to ensure changes make sense
3. **Check that qualifiers don't weaken** the proposal too much
4. **Ensure consistency** across all documents
5. **Update PR description** to note claims are now qualified

---

## Example Before/After

### Before (Problematic)

> OpenFGA edge architecture will achieve <1ms P95 latency and 500K checks/second throughput. The Rust implementation will be 5x faster than Go.

### After (Properly Qualified)

> OpenFGA edge architecture **targets <1ms P95 latency** and **500K checks/second throughput** based on preliminary analysis. The Rust implementation **could potentially** be 3-5x faster than Go, assuming:
> - Optimal implementation following best practices
> - Edge nodes with sufficient memory (8GB+)
> - Working set fits in cache (95%+ hit rate)
>
> **Validation required:** These targets must be confirmed through Phase 1 prototyping (Month 1-2). If prototype achieves <50% of target performance, proposal will be reassessed.

---

## Why This Matters

**Without qualifiers:**
- Claims appear as facts
- Sets expectations that may not be met
- Looks unprofessional to reviewers
- Risk of embarrassment if targets missed

**With qualifiers:**
- Shows honesty and rigor
- Sets realistic expectations
- Provides validation plan
- Demonstrates scientific approach
- Easier to get buy-in (lower perceived risk)

---

## Questions?

See [PR_REVIEW_CHECKLIST.md](./PR_REVIEW_CHECKLIST.md) Blocker B2 for context.
