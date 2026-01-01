# rsfga: Test Document

**Version**: 1.0
**Status**: Draft
**Last Updated**: 2025-12-31

---

## 1. Overview

This document defines test cases for rsfga (Rust FGA), including unit tests, integration tests, and end-to-end tests. All test cases are designed to be compatible with the FGA CLI tool.

---

## 2. Test Model

The following authorization model is used as the primary test model. This model includes:
- Direct user assignments
- Nested group membership (org#member containing org#member)
- Tuple-to-Userset relations (page inheriting from space/parent page)
- Conditions with runtime context evaluation

### 2.1 Model Definition

Save as `model.fga`:

```
model
  schema 1.1

type user

type org
  relations
    define member: [user, org#member]

type space
  relations
    define viewer: [user with external_condition, org#member with external_condition]
    define editor: [user with external_condition, org#member with external_condition]
    define admin: [user with external_condition, org#member with external_condition]
    define can_view: viewer or editor or admin
    define can_edit: editor or admin
    define can_admin: admin

type page
  relations
    define parent: [page, space]
    define can_view: can_view from parent
    define can_edit: can_edit from parent

condition external_condition (external: bool, allow_external: bool) {
  !external || allow_external
}
```

### 2.2 FGA CLI Model YAML

Save as `model.yaml`:

```yaml
model: |
  model
    schema 1.1

  type user

  type org
    relations
      define member: [user, org#member]

  type space
    relations
      define viewer: [user with external_condition, org#member with external_condition]
      define editor: [user with external_condition, org#member with external_condition]
      define admin: [user with external_condition, org#member with external_condition]
      define can_view: viewer or editor or admin
      define can_edit: editor or admin
      define can_admin: admin

  type page
    relations
      define parent: [page, space]
      define can_view: can_view from parent
      define can_edit: can_edit from parent

  condition external_condition (external: bool, allow_external: bool) {
    !external || allow_external
  }
```

---

## 3. Test Data

### 3.1 Tuples File

Save as `tuples.yaml`:

```yaml
tuples:
  # Organization memberships
  - user: user:alice
    relation: member
    object: org:acme

  - user: user:bob
    relation: member
    object: org:acme

  - user: user:charlie
    relation: member
    object: org:widgets

  # Nested org membership (acme is member of enterprise)
  - user: org:acme#member
    relation: member
    object: org:enterprise

  # Space permissions with conditions
  - user: user:alice
    relation: admin
    object: space:engineering
    condition:
      name: external_condition
      context:
        allow_external: true

  - user: org:acme#member
    relation: viewer
    object: space:engineering
    condition:
      name: external_condition
      context:
        allow_external: false

  - user: user:charlie
    relation: editor
    object: space:sales
    condition:
      name: external_condition
      context:
        allow_external: true

  # Page hierarchy
  - user: space:engineering
    relation: parent
    object: page:eng-home

  - user: page:eng-home
    relation: parent
    object: page:eng-docs

  - user: page:eng-docs
    relation: parent
    object: page:eng-api-spec

  - user: space:sales
    relation: parent
    object: page:sales-home
```

---

## 4. Test Cases

### 4.1 FGA CLI Test Format

Save as `tests.yaml`:

```yaml
name: rsfga Test Suite
model_file: model.yaml
tuples_file: tuples.yaml

tests:
  # ============================================
  # Test Suite 1: Direct User Access
  # ============================================
  - name: "Direct admin access - internal"
    check:
      user: user:alice
      object: space:engineering
      relation: can_view
      context:
        external: false
    expected: true

  - name: "Direct admin access - external allowed"
    check:
      user: user:alice
      object: space:engineering
      relation: can_view
      context:
        external: true
    expected: true

  - name: "Direct admin can_edit"
    check:
      user: user:alice
      object: space:engineering
      relation: can_edit
      context:
        external: false
    expected: true

  - name: "Direct admin can_admin"
    check:
      user: user:alice
      object: space:engineering
      relation: can_admin
      context:
        external: false
    expected: true

  # ============================================
  # Test Suite 2: Org Membership Access
  # ============================================
  - name: "Org member viewer access - internal"
    check:
      user: user:bob
      object: space:engineering
      relation: can_view
      context:
        external: false
    expected: true

  - name: "Org member viewer access - external blocked"
    description: "Bob is org:acme#member which has viewer with allow_external=false"
    check:
      user: user:bob
      object: space:engineering
      relation: can_view
      context:
        external: true
    expected: false

  - name: "Org member cannot edit (viewer only)"
    check:
      user: user:bob
      object: space:engineering
      relation: can_edit
      context:
        external: false
    expected: false

  # ============================================
  # Test Suite 3: Nested Org Membership
  # ============================================
  - name: "Nested org member - alice via enterprise"
    description: "alice is in acme, acme#member is in enterprise"
    check:
      user: user:alice
      object: org:enterprise
      relation: member
    expected: true

  - name: "Nested org member - bob via enterprise"
    check:
      user: user:bob
      object: org:enterprise
      relation: member
    expected: true

  - name: "Non-member of enterprise"
    check:
      user: user:charlie
      object: org:enterprise
      relation: member
    expected: false

  # ============================================
  # Test Suite 4: Page Inheritance (TTU)
  # ============================================
  - name: "Page inherits from space - viewer"
    check:
      user: user:bob
      object: page:eng-home
      relation: can_view
      context:
        external: false
    expected: true

  - name: "Nested page inherits from parent page"
    check:
      user: user:bob
      object: page:eng-docs
      relation: can_view
      context:
        external: false
    expected: true

  - name: "Deep nested page (3 levels)"
    check:
      user: user:bob
      object: page:eng-api-spec
      relation: can_view
      context:
        external: false
    expected: true

  - name: "Admin can edit page"
    check:
      user: user:alice
      object: page:eng-home
      relation: can_edit
      context:
        external: false
    expected: true

  - name: "Viewer cannot edit page"
    check:
      user: user:bob
      object: page:eng-home
      relation: can_edit
      context:
        external: false
    expected: false

  # ============================================
  # Test Suite 5: Cross-Space Access Control
  # ============================================
  - name: "No access to other space"
    check:
      user: user:bob
      object: space:sales
      relation: can_view
      context:
        external: false
    expected: false

  - name: "Charlie has editor on sales"
    check:
      user: user:charlie
      object: space:sales
      relation: can_edit
      context:
        external: false
    expected: true

  - name: "Charlie can view sales page"
    check:
      user: user:charlie
      object: page:sales-home
      relation: can_view
      context:
        external: false
    expected: true

  # ============================================
  # Test Suite 6: External Access Control
  # ============================================
  - name: "External access blocked by condition"
    description: "org:acme#member has allow_external=false"
    check:
      user: user:bob
      object: space:engineering
      relation: can_view
      context:
        external: true
    expected: false

  - name: "External access allowed for admin"
    description: "alice has allow_external=true"
    check:
      user: user:alice
      object: space:engineering
      relation: can_view
      context:
        external: true
    expected: true

  - name: "External access for nested page"
    description: "Condition should propagate through TTU"
    check:
      user: user:bob
      object: page:eng-home
      relation: can_view
      context:
        external: true
    expected: false

  - name: "Admin external access to nested page"
    check:
      user: user:alice
      object: page:eng-api-spec
      relation: can_view
      context:
        external: true
    expected: true

  # ============================================
  # Test Suite 7: No Access Cases
  # ============================================
  - name: "Unknown user has no access"
    check:
      user: user:unknown
      object: space:engineering
      relation: can_view
      context:
        external: false
    expected: false

  - name: "Unknown object returns false"
    check:
      user: user:alice
      object: space:nonexistent
      relation: can_view
      context:
        external: false
    expected: false

  - name: "Wrong org member"
    check:
      user: user:charlie
      object: space:engineering
      relation: can_view
      context:
        external: false
    expected: false
```

---

## 5. Running Tests with FGA CLI

### 5.1 Setup

```bash
# Install FGA CLI
brew install openfga/tap/fga

# Or download from GitHub releases
# https://github.com/openfga/cli/releases

# Verify installation
fga version
```

### 5.2 Local Testing

```bash
# Create a local store and run tests
fga model test --tests tests.yaml

# Or step by step:
# 1. Transform model
fga model transform --file model.fga > model.json

# 2. Validate model
fga model validate --file model.json

# 3. Run tests
fga model test --tests tests.yaml --verbose
```

### 5.3 Testing Against Edge Server

```bash
# Set API URL to edge sidecar
export FGA_API_URL=http://localhost:8080

# Create store
fga store create --name "edge-test"

# Write model
fga model write --file model.fga

# Write tuples
fga tuple write --file tuples.yaml

# Run individual checks
fga query check user:alice can_view space:engineering --context '{"external": false}'

# Run all tests
fga model test --tests tests.yaml
```

---

## 6. Edge-Specific Tests

### 6.1 Cache Hit/Miss Tests

These tests verify edge caching behavior.

```yaml
name: Edge Cache Tests
tests:
  - name: "Cache hit - pre-computed result"
    description: "Check should return from edge cache"
    check:
      user: user:alice
      object: space:engineering
      relation: can_view
      context:
        external: false
    expected: true
    edge_expectations:
      cache_hit: true
      latency_p95_ms: 1

  - name: "Cache miss - fallback to central"
    description: "Unknown permission should go to central"
    check:
      user: user:newuser
      object: space:engineering
      relation: can_view
      context:
        external: false
    expected: false
    edge_expectations:
      cache_hit: false
```

### 6.2 Condition Evaluation Tests

```yaml
name: Condition Evaluation Tests
tests:
  - name: "Condition with bound params only"
    description: "allow_external comes from tuple"
    check:
      user: user:alice
      object: space:engineering
      relation: admin
      context:
        external: false
    expected: true
    condition_trace:
      condition: external_condition
      bound_params:
        allow_external: true
      request_context:
        external: false
      result: "!false || true = true"

  - name: "Condition blocks access"
    check:
      user: user:bob
      object: space:engineering
      relation: viewer
      context:
        external: true
    expected: false
    condition_trace:
      condition: external_condition
      bound_params:
        allow_external: false
      request_context:
        external: true
      result: "!true || false = false"
```

### 6.3 Sync Lag Tests

```yaml
name: Sync Lag Tests
tests:
  - name: "Write then immediate check"
    description: "Check after write may be stale"
    steps:
      - write:
          user: user:dave
          relation: viewer
          object: space:engineering
          condition:
            name: external_condition
            context:
              allow_external: true
      - wait: 100ms
      - check:
          user: user:dave
          object: space:engineering
          relation: can_view
          context:
            external: false
        # May fail due to sync lag
        expected: true
        allow_stale: true

  - name: "Write with strong consistency"
    steps:
      - write:
          user: user:eve
          relation: viewer
          object: space:engineering
          condition:
            name: external_condition
            context:
              allow_external: true
      - check:
          user: user:eve
          object: space:engineering
          relation: can_view
          context:
            external: false
          consistency: strong
        expected: true
```

---

## 7. Performance Tests

### 7.1 Latency Benchmarks

```yaml
name: Latency Benchmarks
benchmarks:
  - name: "Simple check latency"
    check:
      user: user:alice
      object: space:engineering
      relation: can_view
      context:
        external: false
    iterations: 10000
    warmup: 1000
    targets:
      p50_us: 100
      p95_us: 500
      p99_us: 1000

  - name: "Check with condition evaluation"
    check:
      user: user:alice
      object: space:engineering
      relation: admin
      context:
        external: true
    iterations: 10000
    warmup: 1000
    targets:
      p50_us: 200
      p95_us: 800
      p99_us: 1500

  - name: "Deep TTU traversal (edge pre-computed)"
    check:
      user: user:alice
      object: page:eng-api-spec
      relation: can_view
      context:
        external: false
    iterations: 10000
    warmup: 1000
    targets:
      p50_us: 100
      p95_us: 500
      p99_us: 1000
```

### 7.2 Throughput Benchmarks

```yaml
name: Throughput Benchmarks
benchmarks:
  - name: "Sustained throughput"
    concurrent_clients: 100
    duration: 60s
    check:
      user: user:alice
      object: space:engineering
      relation: can_view
      context:
        external: false
    targets:
      min_rps: 100000
      max_latency_p99_ms: 2
```

---

## 8. Failure Mode Tests

### 8.1 Edge Failure Scenarios

```yaml
name: Failure Mode Tests
tests:
  - name: "Edge restart recovery"
    steps:
      - check:
          user: user:alice
          object: space:engineering
          relation: can_view
          context:
            external: false
        expected: true
      - action: restart_edge
      - wait: 5s
      - check:
          user: user:alice
          object: space:engineering
          relation: can_view
          context:
            external: false
        expected: true

  - name: "Central unavailable - serve from cache"
    steps:
      - action: block_central
      - check:
          user: user:alice
          object: space:engineering
          relation: can_view
          context:
            external: false
        expected: true
        note: "Served from edge cache"
      - action: unblock_central

  - name: "Central unavailable - cache miss fails"
    steps:
      - action: block_central
      - check:
          user: user:newuser
          object: space:engineering
          relation: can_view
          context:
            external: false
        expected_error: "EDGE_002"
      - action: unblock_central
```

### 8.2 Sync Failure Scenarios

```yaml
name: Sync Failure Tests
tests:
  - name: "Kafka disconnect - continue serving"
    steps:
      - action: disconnect_kafka
      - check:
          user: user:alice
          object: space:engineering
          relation: can_view
          context:
            external: false
        expected: true
        note: "Continues with cached data"
      - wait: 35s
      - check:
          user: user:alice
          object: space:engineering
          relation: can_view
          context:
            external: false
        expected_warning: "EDGE_001"
      - action: reconnect_kafka
```

---

## 9. Integration Test Script

Save as `run-tests.sh`:

```bash
#!/bin/bash
set -e

# Configuration
EDGE_URL="${EDGE_URL:-http://localhost:8080}"
CENTRAL_URL="${CENTRAL_URL:-http://localhost:8081}"
STORE_NAME="${STORE_NAME:-edge-integration-test}"

echo "=== rsfga Integration Tests ==="
echo "Edge URL: $EDGE_URL"
echo "Central URL: $CENTRAL_URL"
echo ""

# 1. Create store on central
echo "Creating store..."
STORE_RESPONSE=$(curl -s -X POST "$CENTRAL_URL/stores" \
  -H "Content-Type: application/json" \
  -d "{\"name\": \"$STORE_NAME\"}")
STORE_ID=$(echo $STORE_RESPONSE | jq -r '.id')
echo "Store ID: $STORE_ID"

# 2. Write model
echo "Writing model..."
fga model write --api-url "$CENTRAL_URL" --store-id "$STORE_ID" --file model.fga

# 3. Write tuples
echo "Writing tuples..."
fga tuple write --api-url "$CENTRAL_URL" --store-id "$STORE_ID" --file tuples.yaml

# 4. Wait for sync
echo "Waiting for edge sync..."
sleep 5

# 5. Run tests against edge
echo "Running tests against edge..."
fga model test --api-url "$EDGE_URL" --store-id "$STORE_ID" --tests tests.yaml

# 6. Verify edge metrics
echo "Checking edge metrics..."
METRICS=$(curl -s "$EDGE_URL/metrics")
echo "Cache hit ratio: $(echo $METRICS | grep 'rsfga_cache_hit_ratio' | awk '{print $2}')"
echo "Sync lag: $(echo $METRICS | grep 'rsfga_sync_lag_seconds' | awk '{print $2}')"

# 7. Cleanup
echo "Cleaning up..."
curl -s -X DELETE "$CENTRAL_URL/stores/$STORE_ID"

echo ""
echo "=== All tests passed! ==="
```

---

## 10. Test Matrix

### 10.1 Feature Coverage

| Feature | Test Cases | Coverage |
|---------|------------|----------|
| Direct user assignment | 4 | ✅ |
| Org membership | 3 | ✅ |
| Nested org membership | 3 | ✅ |
| TTU (page inheritance) | 5 | ✅ |
| Conditions | 4 | ✅ |
| External access control | 4 | ✅ |
| No access cases | 3 | ✅ |
| Cache behavior | 2 | ✅ |
| Sync lag | 2 | ✅ |
| Failure modes | 4 | ✅ |

### 10.2 Relation Coverage

| Relation | Direct | Via Org | Via TTU | With Condition |
|----------|--------|---------|---------|----------------|
| member | ✅ | ✅ | - | - |
| viewer | ✅ | ✅ | ✅ | ✅ |
| editor | ✅ | ✅ | ✅ | ✅ |
| admin | ✅ | - | ✅ | ✅ |
| can_view | ✅ | ✅ | ✅ | ✅ |
| can_edit | ✅ | ✅ | ✅ | ✅ |
| can_admin | ✅ | - | - | ✅ |

---

## Appendix A: Test Data Generation

For large-scale testing, use this script to generate test data:

```python
#!/usr/bin/env python3
"""Generate test tuples for load testing."""

import yaml
import random

def generate_tuples(num_users=1000, num_orgs=10, num_spaces=100, num_pages=1000):
    tuples = []

    # Generate org memberships
    for i in range(num_users):
        org_id = random.randint(0, num_orgs - 1)
        tuples.append({
            "user": f"user:user{i}",
            "relation": "member",
            "object": f"org:org{org_id}"
        })

    # Generate space permissions
    for i in range(num_spaces):
        org_id = i % num_orgs
        tuples.append({
            "user": f"org:org{org_id}#member",
            "relation": "viewer",
            "object": f"space:space{i}",
            "condition": {
                "name": "external_condition",
                "context": {
                    "allow_external": random.choice([True, False])
                }
            }
        })

    # Generate page hierarchy
    for i in range(num_pages):
        space_id = i % num_spaces
        if i < num_spaces:
            # Top-level page
            tuples.append({
                "user": f"space:space{space_id}",
                "relation": "parent",
                "object": f"page:page{i}"
            })
        else:
            # Nested page
            parent_page = random.randint(0, i - 1)
            tuples.append({
                "user": f"page:page{parent_page}",
                "relation": "parent",
                "object": f"page:page{i}"
            })

    return {"tuples": tuples}

if __name__ == "__main__":
    data = generate_tuples()
    print(yaml.dump(data, default_flow_style=False))
```

---

## Appendix B: FGA CLI Quick Reference

```bash
# Model operations
fga model transform --file model.fga          # Convert to JSON
fga model validate --file model.json          # Validate model
fga model write --file model.fga              # Write to server
fga model test --tests tests.yaml             # Run tests

# Tuple operations
fga tuple write --file tuples.yaml            # Write tuples
fga tuple read                                # Read all tuples
fga tuple delete --file tuples.yaml           # Delete tuples

# Query operations
fga query check user:alice can_view doc:1     # Check permission
fga query check user:alice can_view doc:1 \
  --context '{"external": false}'             # With context
fga query list-objects user:alice can_view doc  # List objects
fga query list-users can_view doc:1           # List users

# Store operations
fga store create --name mystore               # Create store
fga store list                                # List stores
fga store delete --store-id <id>              # Delete store
```
