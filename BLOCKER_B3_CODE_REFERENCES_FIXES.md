# Blocker B3: Code References - Required Changes

This document shows how to fix all code references with line numbers.

---

## Problem

Line numbers drift as code changes, making documentation stale. We need to replace line-specific references with function-level references.

**Example Problem:**
```markdown
**Location:** `pkg/server/commands/batch_check_command.go:234`
```

If someone adds/removes lines, line 234 no longer points to the same code.

**Solution:**
```markdown
**Location:** `pkg/server/commands/batch_check_command.go` (function: `BatchCheckQuery.Execute`)
**Note:** Code references accurate as of commit [hash] (2026-01-01)
```

---

## All Fixes Required

### 1. ARCHITECTURE_REVIEW.md

#### Fix 1: Line 119

**CURRENT:**
```markdown
**Location:** `pkg/server/commands/batch_check_command.go:234`
```

**CHANGE TO:**
```markdown
**Location:** `pkg/server/commands/batch_check_command.go` (function: `BatchCheckQuery.Execute`, synchronous wait)
```

#### Fix 2: Line 137 (add new location reference)

**CURRENT:**
```markdown
**Location:** `internal/graph/cached_resolver.go`
```

**CHANGE TO:**
```markdown
**Location:** `internal/graph/cached_resolver.go` (struct: `CachedCheckResolver`, method: `ResolveCheck`)
```

#### Fix 3: Line 153

**CURRENT:**
```markdown
**Location:** `internal/cachecontroller/cache_controller.go:207-260`
```

**CHANGE TO:**
```markdown
**Location:** `internal/cachecontroller/cache_controller.go` (function: `findChangesAndInvalidateIfNecessary`)
```

#### Fix 4: Line 180

**CURRENT:**
```markdown
**Location:** `pkg/server/commands/batch_check_command.go:188-199`
```

**CHANGE TO:**
```markdown
**Location:** `pkg/server/commands/batch_check_command.go` (function: `BatchCheckQuery.Execute`, per-check object allocation)
```

#### Fix 5: Line 192

**CURRENT:**
```markdown
**Location:** `pkg/server/config/config.go:22-23`
```

**CHANGE TO:**
```markdown
**Location:** `pkg/server/config/config.go` (constants: `DefaultResolveNodeLimit`, `DefaultResolveNodeBreadthLimit`)
```

#### Fix 6: Line 209

**CURRENT:**
```markdown
**Location:** `pkg/server/commands/batch_check_command.go:168`
```

**CHANGE TO:**
```markdown
**Location:** `pkg/server/commands/batch_check_command.go` (function: `BatchCheckQuery.Execute`, result collection with `sync.Map`)
```

#### Fix 7: Line 221

**CURRENT:**
```markdown
**Location:** `pkg/server/commands/batch_check_command.go:282-304`
```

**CHANGE TO:**
```markdown
**Location:** `pkg/server/commands/batch_check_command.go` (function: `buildCacheKey`, contextual tuple serialization)
```

#### Fix 8-10: Lines 235-237 (Table)

**CURRENT:**
```markdown
| Response cloning | `cached_resolver.go:176,208` | Minor allocation overhead |
| Hash computation per check | `batch_check_command.go:150` | CPU overhead for deduplication |
| Resolver chain construction | `batch_check.go:56-61` | Per-request overhead |
```

**CHANGE TO:**
```markdown
| Response cloning | `internal/graph/cached_resolver.go` (method: `ResolveCheck`, response cloning) | Minor allocation overhead |
| Hash computation per check | `pkg/server/commands/batch_check_command.go` (function: `buildCacheKey`) | CPU overhead for deduplication |
| Resolver chain construction | `pkg/server/batch_check.go` (function: `NewBatchCheckCommand`, resolver chain setup) | Per-request overhead |
```

### 2. Add Git Commit Reference

**ADD TO ARCHITECTURE_REVIEW.md AFTER TITLE:**

```markdown
# OpenFGA Architecture Review: Scalability, Performance & Reimplementation Analysis

> **Code References:** All code file and function references in this document are accurate as of:
> - **Git Commit:** `[INSERT CURRENT COMMIT HASH]`
> - **Date:** 2026-01-01
> - **Branch:** `main`
>
> Code may have changed since this analysis. Use references as guidance, not exact line numbers.

---

## Executive Summary
...
```

### 3. BATCH_CHECK_ANALYSIS.md (if exists)

Similar pattern - replace all `:line_number` references with function names.

---

## Automated Fix Script

Create `scripts/fix-code-references.sh`:

```bash
#!/bin/bash
# Fix all code references to use function names instead of line numbers

set -e

echo "Fixing code references in ARCHITECTURE_REVIEW.md..."

# Get current commit hash
COMMIT_HASH=$(git rev-parse --short HEAD)
CURRENT_DATE=$(date +%Y-%m-%d)

# Fix 1: Line 119
sed -i.bak 's|pkg/server/commands/batch_check_command\.go:234|pkg/server/commands/batch_check_command.go` (function: `BatchCheckQuery.Execute`, synchronous wait)|g' ARCHITECTURE_REVIEW.md

# Fix 2: Line 137
sed -i.bak 's|internal/graph/cached_resolver\.go`|internal/graph/cached_resolver.go` (struct: `CachedCheckResolver`, method: `ResolveCheck`)|g' ARCHITECTURE_REVIEW.md

# Fix 3: Line 153
sed -i.bak 's|internal/cachecontroller/cache_controller\.go:207-260|internal/cachecontroller/cache_controller.go` (function: `findChangesAndInvalidateIfNecessary`)|g' ARCHITECTURE_REVIEW.md

# Fix 4: Line 180
sed -i.bak 's|pkg/server/commands/batch_check_command\.go:188-199|pkg/server/commands/batch_check_command.go` (function: `BatchCheckQuery.Execute`, per-check allocation)|g' ARCHITECTURE_REVIEW.md

# Fix 5: Line 192
sed -i.bak 's|pkg/server/config/config\.go:22-23|pkg/server/config/config.go` (constants: `DefaultResolveNodeLimit`, `DefaultResolveNodeBreadthLimit`)|g' ARCHITECTURE_REVIEW.md

# Fix 6: Line 209
sed -i.bak 's|batch_check_command\.go:168`|batch_check_command.go` (function: `BatchCheckQuery.Execute`, sync.Map usage)|g' ARCHITECTURE_REVIEW.md

# Fix 7: Line 221
sed -i.bak 's|batch_check_command\.go:282-304|batch_check_command.go` (function: `buildCacheKey`)|g' ARCHITECTURE_REVIEW.md

# Fix 8: cached_resolver line references
sed -i.bak 's|cached_resolver\.go:176,208|internal/graph/cached_resolver.go` (method: `ResolveCheck`)|g' ARCHITECTURE_REVIEW.md

# Fix 9: hash computation reference
sed -i.bak 's|batch_check_command\.go:150`|batch_check_command.go` (function: `buildCacheKey`)|g' ARCHITECTURE_REVIEW.md

# Fix 10: resolver chain
sed -i.bak 's|batch_check\.go:56-61|batch_check.go` (function: `NewBatchCheckCommand`)|g' ARCHITECTURE_REVIEW.md

# Add commit reference header
HEADER="> **Code References:** All code file and function references in this document are accurate as of:\\\\
> - **Git Commit:** \`$COMMIT_HASH\`\\\\
> - **Date:** $CURRENT_DATE\\\\
> - **Branch:** \`main\`\\\\
>\\\\
> Code may have changed since this analysis. Use references as guidance, not exact line numbers.\\\\
\\\\
---"

sed -i.bak "3i\\
$HEADER
" ARCHITECTURE_REVIEW.md

# Clean up backups
find . -name "*.md.bak" -delete

echo "✅ Code references fixed!"
echo ""
echo "Changes made:"
echo "  - Replaced line numbers with function/method names"
echo "  - Added commit reference: $COMMIT_HASH"
echo "  - Added date reference: $CURRENT_DATE"
echo ""
echo "Next: Review with 'git diff ARCHITECTURE_REVIEW.md'"
```

---

## Manual Verification Checklist

After running the script, manually verify:

- [ ] All `:line_number` patterns removed
- [ ] Function/method names are accurate (spot check in actual code)
- [ ] Commit hash is current
- [ ] Date is correct
- [ ] No broken markdown formatting
- [ ] References still make sense in context

---

## How to Verify Function Names are Correct

Before finalizing, spot-check a few references:

```bash
# Example: Verify batch_check_command.go has BatchCheckQuery.Execute
grep -n "func.*Execute" pkg/server/commands/batch_check_command.go

# Example: Verify cached_resolver.go has ResolveCheck
grep -n "func.*ResolveCheck" internal/graph/cached_resolver.go

# Example: Verify config.go has those constants
grep -n "DefaultResolveNode" pkg/server/config/config.go
```

If any don't match:
1. Read the actual file
2. Find the correct function name
3. Update the reference manually

---

## Why This Matters

### Problem with Line Numbers

```markdown
**Before (2026-01-01):**
Line 234: `_ = pool.Wait()`  ← Correct

**After someone adds 10 lines (2026-02-01):**
Line 234: `results := make(map[string]bool)`  ← WRONG! Now points to different code

**Reader follows reference:**
"This document says Wait() is at line 234, but I see make() there. Is this doc outdated?"
```

### Solution with Function Names

```markdown
**Always:**
Function `BatchCheckQuery.Execute` has `pool.Wait()` call

**Reader follows reference:**
1. Opens batch_check_command.go
2. Searches for "func.*Execute"
3. Finds correct function, regardless of line number changes
4. Sees code in context
```

---

## Alternative: Add Line Numbers as Comments

If you want to keep line numbers for convenience:

```markdown
**Location:** `pkg/server/commands/batch_check_command.go` (function: `BatchCheckQuery.Execute`)
<!-- As of commit abc123, this was around line 234 -->
```

Benefits:
- Gives readers a hint where to look
- Comment clearly indicates it's approximate
- Won't confuse readers if number is off

---

## After Applying Fixes

1. **Test the references** by actually opening the files and finding the functions
2. **Update commit hash** if you've made changes since last commit
3. **Document when references were checked** in commit message:

```bash
git add ARCHITECTURE_REVIEW.md
git commit -m "Fix code references to use function names (Blocker B3)

- Replaced all line number references with function/method names
- Added commit reference header
- References accurate as of commit $(git rev-parse --short HEAD)
- Manually verified function names in spot checks"
```

---

## Examples of Good vs Bad References

### ❌ Bad (Will Drift)

```markdown
**Location:** `server.go:123`
**Location:** `handler.go:45-67`
**Location:** `types.go:234, 456, 789`
```

### ✅ Good (Stable)

```markdown
**Location:** `pkg/server/server.go` (function: `NewServer`)
**Location:** `pkg/handler/handler.go` (method: `HandleRequest`)
**Location:** `pkg/types/types.go` (struct: `AuthModel`, fields: `TypeDefinitions`)
```

### ⭐ Best (With Context)

```markdown
**Location:** `pkg/server/commands/batch_check_command.go`
- **Function:** `BatchCheckQuery.Execute`
- **Issue:** Synchronous `pool.Wait()` blocks until all checks complete
- **Impact:** Batch latency = max(individual check latencies)
```

---

## Questions?

See [PR_REVIEW_CHECKLIST.md](./PR_REVIEW_CHECKLIST.md) Blocker B3 for context.
