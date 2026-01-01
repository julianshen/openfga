#!/bin/bash
# Apply performance claim qualifiers to all architecture documents
# Part of Blocker B2 fixes for PR #6

set -e  # Exit on error

echo "=========================================="
echo "Applying Performance Claim Qualifiers"
echo "=========================================="
echo ""

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Step 1: Update ARCHITECTURE_REVIEW.md - Line 11
echo -e "${YELLOW}[1/7]${NC} Updating ARCHITECTURE_REVIEW.md..."
sed -i.bak 's/- A Rust reimplementation could yield 2-5x performance improvements with proper design/- A Rust reimplementation could **potentially** yield 2-5x performance improvements with proper design **based on preliminary analysis** (requires validation)/' ARCHITECTURE_REVIEW.md

# Step 2: Add Performance Assumptions section to ARCHITECTURE_REVIEW.md
echo -e "${YELLOW}[2/7]${NC} Adding Performance Assumptions section..."
# Insert after line 13 (after "Storage backend choice..." line)
sed -i.bak '13 a\
\
---\
\
## ⚠️ Performance Assumptions & Disclaimers\
\
**IMPORTANT:** All performance projections in this document are **estimates based on preliminary analysis**. They have NOT been validated through prototyping or benchmarking.\
\
### Assumptions\
\
These performance targets assume:\
\
1. **Hardware:** Edge nodes with 8GB+ RAM, modern CPUs (2020+)\
2. **Network:** <0.1ms latency between application and edge (localhost deployment)\
3. **Data Characteristics:**\
   - Authorization models are not pathologically deep (depth <10)\
   - Working set fits in edge memory (95%+ cache hit rate)\
   - Pre-computation keeps up with write rate\
4. **Workload:** Read-heavy (90%+ reads vs writes)\
5. **Implementation:** Optimal Rust code following best practices\
\
### Validation Plan\
\
**Phase 1 (Month 1-2):** Build prototype and benchmark\
- [ ] Validate 2-5x improvement claim\
- [ ] Measure actual P95/P99 latencies\
- [ ] Test with production-like data\
\
**Phase 2 (Month 3-4):** Load testing\
- [ ] Test under concurrent load\
- [ ] Measure GC pause elimination\
- [ ] Validate throughput claims\
\
**Success Criteria:**\
- ✅ P95 <1ms achieved in prototype\
- ✅ Throughput >100K checks/s per node\
- ✅ No degradation under load\
\
**If validation fails:** Reassess approach or abandon proposal\
\
### Comparable Systems\
\
Performance claims are informed by:\
- **Rust vs Go benchmarks:** https://benchmarksgame-team.pages.debian.net/benchmarksgame/\
- **DashMap benchmarks:** Concurrent HashMap performance data\
- **Similar systems:** AWS Verified Permissions (Rust), Authzed (Go)\
\
' ARCHITECTURE_REVIEW.md

# Step 3: Update SUB_MS_CHECK_DESIGN.md
echo -e "${YELLOW}[3/7]${NC} Updating docs/architecture/SUB_MS_CHECK_DESIGN.md..."
sed -i.bak 's/**Goal**: Achieve <1ms P95 latency for Check operations at edge nodes\./**Goal**: Achieve **target of <1ms P95 latency** for Check operations at edge nodes.\
\
**Status:** ⚠️ **Unvalidated Target** - Requires prototyping to confirm achievability/' docs/architecture/SUB_MS_CHECK_DESIGN.md

# Step 4: Update 01_DESIGN_DOCUMENT.md
echo -e "${YELLOW}[4/7]${NC} Updating docs/edge/01_DESIGN_DOCUMENT.md..."
sed -i.bak 's/| Check latency P95 | <1ms | /| Check latency P95 | **Target: <1ms** | /' docs/edge/01_DESIGN_DOCUMENT.md
sed -i.bak 's/| Check latency P99 | <2ms | /| Check latency P99 | **Target: <2ms** | /' docs/edge/01_DESIGN_DOCUMENT.md
sed -i.bak 's/| Throughput per edge | 500K checks\/s | /| Throughput per edge | **Target: 500K checks\/s** | /' docs/edge/01_DESIGN_DOCUMENT.md
sed -i.bak 's/| API compatibility | 100% | /| API compatibility | **Goal: 100%** | /' docs/edge/01_DESIGN_DOCUMENT.md

# Step 5: Add disclaimer to top of each architecture document
echo -e "${YELLOW}[5/7]${NC} Adding disclaimers to architecture documents..."
DISCLAIMER='---\
> ### 📊 Performance Claims Disclaimer\
>\
> This document contains **performance projections and targets** that are:\
> - ✅ Based on preliminary analysis and research\
> - ✅ Informed by similar systems and benchmarks\
> - ❌ NOT validated through prototyping\
> - ❌ NOT guaranteed to be achievable\
>\
> **All performance claims require validation** through the phased prototyping and testing approach described in the RFC.\
>\
> See RFC-001 Performance Assumptions section for full details.\
---\
'

for file in docs/architecture/EDGE_ARCHITECTURE.md docs/architecture/PRECOMPUTATION_ENGINE.md docs/architecture/SUB_MS_CHECK_DESIGN.md; do
    if [ -f "$file" ]; then
        # Insert after first line (title)
        sed -i.bak "2i\\
$DISCLAIMER
" "$file"
        echo "  ✓ Added disclaimer to $file"
    fi
done

# Step 6: Global replacements for qualifying language
echo -e "${YELLOW}[6/7]${NC} Applying global qualifiers..."

# Replace "will achieve" with "targets"
find docs/architecture docs/edge -name "*.md" -type f -exec sed -i.bak 's/will achieve/targets/g' {} \;

# Replace "can achieve" with "may achieve"
find docs/architecture docs/edge -name "*.md" -type f -exec sed -i.bak 's/can achieve/may achieve/g' {} \;

# Step 7: Clean up backup files
echo -e "${YELLOW}[7/7]${NC} Cleaning up backup files..."
find . -name "*.md.bak" -delete

echo ""
echo -e "${GREEN}=========================================="
echo "✅ Performance Qualifiers Applied!"
echo -e "==========================================${NC}"
echo ""
echo "Next steps:"
echo "1. Review changes: git diff"
echo "2. Test build: make sure docs still render correctly"
echo "3. Read through modified sections for readability"
echo "4. Commit changes: git add . && git commit -m 'Add qualifiers to performance claims (Blocker B2)'"
echo ""
echo "Files modified:"
echo "  - ARCHITECTURE_REVIEW.md"
echo "  - docs/architecture/SUB_MS_CHECK_DESIGN.md"
echo "  - docs/architecture/EDGE_ARCHITECTURE.md"
echo "  - docs/architecture/PRECOMPUTATION_ENGINE.md"
echo "  - docs/edge/01_DESIGN_DOCUMENT.md"
echo ""
