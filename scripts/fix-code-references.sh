#!/bin/bash
# Fix all code references to use function names instead of line numbers
# Part of Blocker B3 fixes for PR #6

set -e

echo "=========================================="
echo "Fixing Code References"
echo "=========================================="
echo ""

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Get current commit hash and date
COMMIT_HASH=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
CURRENT_DATE=$(date +%Y-%m-%d)

echo "Commit: $COMMIT_HASH"
echo "Date: $CURRENT_DATE"
echo ""

# Fix ARCHITECTURE_REVIEW.md
echo -e "${YELLOW}[1/3]${NC} Fixing ARCHITECTURE_REVIEW.md code references..."

# Fix specific line number references
sed -i.bak \
    -e 's|pkg/server/commands/batch_check_command\.go:234|pkg/server/commands/batch_check_command.go\` (function: \`BatchCheckQuery.Execute\`, synchronous wait)|g' \
    -e 's|internal/graph/cached_resolver\.go\`$|internal/graph/cached_resolver.go\` (struct: \`CachedCheckResolver\`, method: \`ResolveCheck\`)|g' \
    -e 's|internal/cachecontroller/cache_controller\.go:207-260|internal/cachecontroller/cache_controller.go\` (function: \`findChangesAndInvalidateIfNecessary\`)|g' \
    -e 's|pkg/server/commands/batch_check_command\.go:188-199|pkg/server/commands/batch_check_command.go\` (function: \`BatchCheckQuery.Execute\`, per-check allocation)|g' \
    -e 's|pkg/server/config/config\.go:22-23|pkg/server/config/config.go\` (constants: \`DefaultResolveNodeLimit\`, \`DefaultResolveNodeBreadthLimit\`)|g' \
    -e 's|batch_check_command\.go:168\`|batch_check_command.go\` (function: \`BatchCheckQuery.Execute\`, sync.Map usage)|g' \
    -e 's|batch_check_command\.go:282-304|batch_check_command.go\` (function: \`buildCacheKey\`)|g' \
    -e 's|cached_resolver\.go:176,208|internal/graph/cached_resolver.go\` (method: \`ResolveCheck\`)|g' \
    -e 's|batch_check_command\.go:150\`|batch_check_command.go\` (function: \`buildCacheKey\`)|g' \
    -e 's|batch_check\.go:56-61|batch_check.go\` (function: \`NewBatchCheckCommand\`)|g' \
    ARCHITECTURE_REVIEW.md

# Add commit reference header (after first title line)
echo -e "${YELLOW}[2/3]${NC} Adding commit reference header..."

# Create temporary file with header
cat > /tmp/code_ref_header.txt << EOF

> **Code References:** All code file and function references in this document are accurate as of:
> - **Git Commit:** \`$COMMIT_HASH\`
> - **Date:** $CURRENT_DATE
> - **Branch:** \`claude/rsfga-edge-design-nY8qo\`
>
> Code may have changed since this analysis. Use references as guidance, not exact line numbers.

---
EOF

# Insert header after line 1 (title)
sed -i.bak '1r /tmp/code_ref_header.txt' ARCHITECTURE_REVIEW.md

# Clean up
rm /tmp/code_ref_header.txt

# Fix any similar references in other docs
echo -e "${YELLOW}[3/3]${NC} Checking other documents..."

# If BATCH_CHECK_ANALYSIS.md exists, fix it too
if [ -f "BATCH_CHECK_ANALYSIS.md" ]; then
    echo "  Fixing BATCH_CHECK_ANALYSIS.md..."
    sed -i.bak \
        -e 's|batch_check_command\.go:[0-9-]*|batch_check_command.go` (see ARCHITECTURE_REVIEW.md for function references)|g' \
        -e 's|cache_controller\.go:[0-9-]*|cache_controller.go` (see ARCHITECTURE_REVIEW.md for function references)|g' \
        -e 's|cached_resolver\.go:[0-9,]*|cached_resolver.go` (see ARCHITECTURE_REVIEW.md for function references)|g' \
        BATCH_CHECK_ANALYSIS.md
    echo "  ✓ BATCH_CHECK_ANALYSIS.md updated"
fi

# Clean up all backup files
find . -name "*.md.bak" -delete

echo ""
echo -e "${GREEN}=========================================="
echo "✅ Code References Fixed!"
echo -e "==========================================${NC}"
echo ""
echo "Changes made:"
echo "  ✓ Replaced line numbers with function/method names"
echo "  ✓ Added commit reference: $COMMIT_HASH"
echo "  ✓ Added date: $CURRENT_DATE"
echo ""
echo "Files modified:"
echo "  - ARCHITECTURE_REVIEW.md"
if [ -f "BATCH_CHECK_ANALYSIS.md" ]; then
    echo "  - BATCH_CHECK_ANALYSIS.md"
fi
echo ""
echo "Next steps:"
echo "1. Review changes: git diff ARCHITECTURE_REVIEW.md"
echo "2. Spot-check function names are correct (see BLOCKER_B3_CODE_REFERENCES_FIXES.md)"
echo "3. Commit: git add . && git commit -m 'Fix code references (Blocker B3)'"
echo ""
