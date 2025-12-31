# Pre-Computation Engine: From Tuples to Check Results

## 1. The Core Problem

When a tuple changes, we need to determine **all affected check results** and re-compute them.

```
Tuple Write: document:doc1#viewer@user:alice
                    ↓
       Which checks are affected?
                    ↓
       (document:doc1, viewer, user:alice) → allowed: true

But what about:
  - Computed relations? (viewer from editor)
  - Inherited permissions? (viewer from parent folder)
  - Group memberships? (alice is in team:engineering)
```

---

## 2. Relationship Types and Affected Checks

### 2.1 Direct Assignment (Simple)

```yaml
# Model
type document
  relations
    define viewer: [user]
```

```
Tuple: document:doc1#viewer@user:alice

Affected Checks: Only 1
  → (document:doc1, viewer, user:alice) = true
```

**Computation**: O(1) - trivial

---

### 2.2 Computed Userset (Medium Complexity)

```yaml
# Model
type document
  relations
    define editor: [user]
    define viewer: editor  # viewer = anyone who is editor
```

```
Tuple: document:doc1#editor@user:alice

Affected Checks: 2
  → (document:doc1, editor, user:alice) = true
  → (document:doc1, viewer, user:alice) = true  # because viewer := editor
```

**Computation**: Need to traverse model to find all relations that include this one

---

### 2.3 Tuple-to-Userset / TTU (Complex)

```yaml
# Model
type folder
  relations
    define viewer: [user]

type document
  relations
    define parent: [folder]
    define viewer: [user] or viewer from parent  # inherit from parent folder
```

```
Tuple: folder:folder1#viewer@user:alice

Affected Checks: 1 + N (where N = documents with parent=folder1)
  → (folder:folder1, viewer, user:alice) = true
  → (document:doc1, viewer, user:alice) = true   # if doc1.parent = folder1
  → (document:doc2, viewer, user:alice) = true   # if doc2.parent = folder1
  → ... for all documents under folder1
```

**Computation**: Need reverse lookup to find all objects pointing to this one

---

### 2.4 Group/Team Membership (Most Complex)

```yaml
# Model
type team
  relations
    define member: [user]

type document
  relations
    define viewer: [user, team#member]
```

```
Tuple: team:engineering#member@user:alice

Affected Checks: 1 + M (where M = documents with viewer=team:engineering#member)
  → (team:engineering, member, user:alice) = true
  → (document:doc1, viewer, user:alice) = true   # if doc1 has viewer = team:engineering#member
  → (document:doc2, viewer, user:alice) = true   # if doc2 has viewer = team:engineering#member
  → ...
```

---

## 3. Pre-Computation Algorithm

### 3.1 Overview

```
┌─────────────────────────────────────────────────────────────────────────────────────┐
│                         PRE-COMPUTATION PIPELINE                                    │
│                                                                                     │
│   Tuple Change                                                                      │
│       │                                                                             │
│       ▼                                                                             │
│   ┌─────────────────────────────────────────────────────────────────────────────┐  │
│   │  STEP 1: Classify Change Type                                               │  │
│   │                                                                             │  │
│   │  • Direct assignment?                                                       │  │
│   │  • Group membership?                                                        │  │
│   │  • Parent/container relationship?                                           │  │
│   │  • Userset reference?                                                       │  │
│   └─────────────────────────────────────────────────────────────────────────────┘  │
│       │                                                                             │
│       ▼                                                                             │
│   ┌─────────────────────────────────────────────────────────────────────────────┐  │
│   │  STEP 2: Find Affected Objects (Reverse Expansion)                          │  │
│   │                                                                             │  │
│   │  Query: "What objects reference this tuple's subject?"                      │  │
│   │                                                                             │  │
│   │  Example: If team:eng#member@alice was added,                               │  │
│   │           find all objects with viewer = team:eng#member                    │  │
│   └─────────────────────────────────────────────────────────────────────────────┘  │
│       │                                                                             │
│       ▼                                                                             │
│   ┌─────────────────────────────────────────────────────────────────────────────┐  │
│   │  STEP 3: Find Affected Users (Forward Expansion)                            │  │
│   │                                                                             │  │
│   │  For container changes: "What users are affected?"                          │  │
│   │                                                                             │  │
│   │  Example: If folder:f1#viewer@team:eng#member was added,                    │  │
│   │           find all users who are team:eng#member                            │  │
│   └─────────────────────────────────────────────────────────────────────────────┘  │
│       │                                                                             │
│       ▼                                                                             │
│   ┌─────────────────────────────────────────────────────────────────────────────┐  │
│   │  STEP 4: Compute Check Results                                              │  │
│   │                                                                             │  │
│   │  For each (object, relation, user) combination:                             │  │
│   │    result = full_check(object, relation, user)                              │  │
│   │    publish(object, relation, user, result)                                  │  │
│   └─────────────────────────────────────────────────────────────────────────────┘  │
│       │                                                                             │
│       ▼                                                                             │
│   ┌─────────────────────────────────────────────────────────────────────────────┐  │
│   │  STEP 5: Publish to Edges                                                   │  │
│   │                                                                             │  │
│   │  • Filter by store_id → which edges subscribe                               │  │
│   │  • Batch updates for efficiency                                             │  │
│   │  • Include deletion markers for removed permissions                         │  │
│   └─────────────────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────────────────┘
```

### 3.2 Detailed Algorithm (Rust)

```rust
use std::collections::HashSet;

/// Represents a tuple change from the changelog
pub struct TupleChange {
    pub store_id: String,
    pub object_type: String,
    pub object_id: String,
    pub relation: String,
    pub user_type: String,
    pub user_id: String,
    pub user_relation: Option<String>,  // For userset subjects like team#member
    pub operation: Operation,           // Write or Delete
    pub ulid: String,
}

/// Pre-computation engine
pub struct PreComputeEngine {
    datastore: Arc<dyn OpenFGADatastore>,
    type_system: Arc<TypeSystemResolver>,
    check_resolver: Arc<dyn CheckResolver>,
    publisher: Arc<dyn SyncPublisher>,
}

impl PreComputeEngine {
    /// Main entry point: process a tuple change
    pub async fn process_change(&self, change: &TupleChange) -> Result<(), Error> {
        let model = self.type_system.get_model(&change.store_id).await?;

        // Step 1: Classify the change
        let change_type = self.classify_change(&change, &model);

        // Step 2 & 3: Find all affected (object, relation, user) combinations
        let affected_checks = self.find_affected_checks(&change, &model, change_type).await?;

        // Step 4: Compute results for each affected check
        let mut results = Vec::with_capacity(affected_checks.len());
        for check_key in affected_checks {
            let result = self.compute_check(&check_key).await?;
            results.push(CheckResultUpdate {
                key_hash: hash_check_key(&check_key),
                object: check_key.object.clone(),
                relation: check_key.relation.clone(),
                user: check_key.user.clone(),
                allowed: result.allowed,
                deleted: change.operation == Operation::Delete && !result.allowed,
                computed_at: now_micros(),
            });
        }

        // Step 5: Publish to edges
        self.publisher.publish(CheckResultSync {
            store_id: change.store_id.clone(),
            updates: results,
            watermark: change.ulid.clone(),
        }).await?;

        Ok(())
    }

    /// Classify what type of change this is
    fn classify_change(&self, change: &TupleChange, model: &AuthModel) -> ChangeType {
        // Check if the subject is a userset (e.g., team#member)
        if change.user_relation.is_some() {
            return ChangeType::UsersetAssignment;
        }

        // Check if this relation is used in TTU by other types
        let is_tupleset = model.relations_used_as_tupleset(&change.object_type, &change.relation);
        if is_tupleset {
            return ChangeType::TuplesetChange;
        }

        // Check if this is a computed userset source
        let is_computed_source = model.is_computed_userset_source(&change.object_type, &change.relation);
        if is_computed_source {
            return ChangeType::ComputedUsersetSource;
        }

        ChangeType::DirectAssignment
    }

    /// Find all affected check combinations
    async fn find_affected_checks(
        &self,
        change: &TupleChange,
        model: &AuthModel,
        change_type: ChangeType,
    ) -> Result<Vec<CheckKey>, Error> {
        let mut affected = HashSet::new();

        // Always include the direct check
        affected.insert(CheckKey {
            object: format!("{}:{}", change.object_type, change.object_id),
            relation: change.relation.clone(),
            user: self.format_user(change),
        });

        match change_type {
            ChangeType::DirectAssignment => {
                // Also check computed relations that include this one
                self.add_computed_relations(&mut affected, change, model);
            }

            ChangeType::ComputedUsersetSource => {
                // Find all relations computed from this one
                self.add_computed_relations(&mut affected, change, model);
            }

            ChangeType::UsersetAssignment => {
                // User added to group - find all objects granting access to this group
                self.add_userset_affected(&mut affected, change, model).await?;
            }

            ChangeType::TuplesetChange => {
                // Parent/container changed - find all objects and users affected
                self.add_ttu_affected(&mut affected, change, model).await?;
            }
        }

        Ok(affected.into_iter().collect())
    }

    /// Add checks for computed usersets (e.g., viewer := editor)
    fn add_computed_relations(
        &self,
        affected: &mut HashSet<CheckKey>,
        change: &TupleChange,
        model: &AuthModel,
    ) {
        // Find relations that compute from this one
        // e.g., if change is to "editor", find "viewer" if viewer := editor
        let computed_from = model.get_relations_computed_from(
            &change.object_type,
            &change.relation,
        );

        for relation in computed_from {
            affected.insert(CheckKey {
                object: format!("{}:{}", change.object_type, change.object_id),
                relation,
                user: self.format_user(change),
            });
        }
    }

    /// Add checks affected by userset assignment (e.g., user added to team)
    async fn add_userset_affected(
        &self,
        affected: &mut HashSet<CheckKey>,
        change: &TupleChange,
        model: &AuthModel,
    ) -> Result<(), Error> {
        // change: team:engineering#member@user:alice
        // Need to find: all objects with viewer/editor/etc = team:engineering#member

        let userset_subject = format!(
            "{}:{}#{}",
            change.object_type,
            change.object_id,
            change.relation
        );

        // Find all types that can have this userset as a subject
        let referencing_types = model.get_types_referencing_userset(
            &change.object_type,
            &change.relation,
        );

        for (object_type, relations) in referencing_types {
            for relation in relations {
                // Query: find all objects of this type with relation = userset_subject
                let objects = self.datastore.read_objects_with_user(
                    &change.store_id,
                    &object_type,
                    &relation,
                    &userset_subject,
                ).await?;

                // The actual user is the one being added to the team
                let actual_user = self.format_user(change);

                for obj in objects {
                    affected.insert(CheckKey {
                        object: obj,
                        relation: relation.clone(),
                        user: actual_user.clone(),
                    });

                    // Also add computed relations
                    for computed in model.get_relations_computed_from(&object_type, &relation) {
                        affected.insert(CheckKey {
                            object: obj.clone(),
                            relation: computed,
                            user: actual_user.clone(),
                        });
                    }
                }
            }
        }

        Ok(())
    }

    /// Add checks affected by TTU changes (e.g., document's parent folder changed)
    async fn add_ttu_affected(
        &self,
        affected: &mut HashSet<CheckKey>,
        change: &TupleChange,
        model: &AuthModel,
    ) -> Result<(), Error> {
        // Two scenarios:
        // 1. Tupleset relation changed (e.g., document#parent@folder:f1)
        //    → Need to find all users with access through this folder
        // 2. Permission on container changed (e.g., folder:f1#viewer@user:alice)
        //    → Need to find all objects that inherit from this container

        // Scenario 1: Tupleset change
        if self.is_tupleset_relation(change, model) {
            // Find users who have access through the container
            let container = format!("{}:{}", change.user_type, change.user_id);
            let users = self.find_users_through_container(&container, model).await?;

            for (relation, user) in users {
                affected.insert(CheckKey {
                    object: format!("{}:{}", change.object_type, change.object_id),
                    relation,
                    user,
                });
            }
        }

        // Scenario 2: Container permission change
        let inheriting_types = model.get_types_inheriting_from(
            &change.object_type,
            &change.relation,
        );

        for (child_type, tupleset_relation, target_relation) in inheriting_types {
            // Find all objects that have this container as their tupleset
            let container = format!("{}:{}", change.object_type, change.object_id);
            let children = self.datastore.read_objects_with_user(
                &change.store_id,
                &child_type,
                &tupleset_relation,
                &container,
            ).await?;

            for child in children {
                affected.insert(CheckKey {
                    object: child,
                    relation: target_relation.clone(),
                    user: self.format_user(change),
                });
            }
        }

        Ok(())
    }

    /// Compute a single check result
    async fn compute_check(&self, key: &CheckKey) -> Result<CheckResponse, Error> {
        self.check_resolver.resolve(CheckRequest {
            store_id: key.store_id.clone(),
            object: key.object.clone(),
            relation: key.relation.clone(),
            user: key.user.clone(),
            contextual_tuples: vec![],
            context: None,
        }).await
    }

    fn format_user(&self, change: &TupleChange) -> String {
        match &change.user_relation {
            Some(rel) => format!("{}:{}#{}", change.user_type, change.user_id, rel),
            None => format!("{}:{}", change.user_type, change.user_id),
        }
    }
}

#[derive(Debug, Clone, PartialEq, Eq)]
enum ChangeType {
    DirectAssignment,      // user directly assigned
    ComputedUsersetSource, // relation used in computed userset
    UsersetAssignment,     // user added to group/team
    TuplesetChange,        // container relationship changed
}
```

---

## 4. Worked Examples

### Example 1: Direct Assignment

```
Model:
  type document
    relations
      define viewer: [user]
      define editor: [user]
      define owner: [user]

Tuple Write: document:doc1#viewer@user:alice

Pre-computation:
  1. Classify: DirectAssignment
  2. Affected checks: [(document:doc1, viewer, user:alice)]
  3. Compute: check(document:doc1, viewer, user:alice) → true
  4. Publish: {key: hash(...), allowed: true}

Result: 1 check computed
```

### Example 2: Computed Userset

```
Model:
  type document
    relations
      define editor: [user]
      define viewer: editor or [user]  # viewer includes all editors

Tuple Write: document:doc1#editor@user:alice

Pre-computation:
  1. Classify: ComputedUsersetSource (editor is source for viewer)
  2. Affected checks:
     - (document:doc1, editor, user:alice)  # direct
     - (document:doc1, viewer, user:alice)  # computed from editor
  3. Compute both checks
  4. Publish: 2 results

Result: 2 checks computed
```

### Example 3: Team Membership (Userset)

```
Model:
  type team
    relations
      define member: [user]

  type document
    relations
      define viewer: [user, team#member]

Existing tuples:
  - document:doc1#viewer@team:engineering#member
  - document:doc2#viewer@team:engineering#member
  - document:doc3#viewer@team:sales#member

Tuple Write: team:engineering#member@user:alice

Pre-computation:
  1. Classify: UsersetAssignment
  2. Find objects with viewer = team:engineering#member
     → [document:doc1, document:doc2]
  3. Affected checks:
     - (team:engineering, member, user:alice)  # direct
     - (document:doc1, viewer, user:alice)     # through team
     - (document:doc2, viewer, user:alice)     # through team
  4. Compute all 3 checks
  5. Publish: 3 results

Result: 3 checks computed (scales with # of documents assigned to team)
```

### Example 4: Folder Inheritance (TTU)

```
Model:
  type user

  type folder
    relations
      define viewer: [user]

  type document
    relations
      define parent: [folder]
      define viewer: [user] or viewer from parent

Existing tuples:
  - document:doc1#parent@folder:folder1
  - document:doc2#parent@folder:folder1
  - document:doc3#parent@folder:folder2

Tuple Write: folder:folder1#viewer@user:alice

Pre-computation:
  1. Classify: TuplesetChange (folder#viewer used in TTU)
  2. Find documents with parent = folder:folder1
     → [document:doc1, document:doc2]
  3. Affected checks:
     - (folder:folder1, viewer, user:alice)    # direct
     - (document:doc1, viewer, user:alice)     # inherited
     - (document:doc2, viewer, user:alice)     # inherited
  4. Compute all 3 checks
  5. Publish: 3 results

Result: 3 checks computed
```

### Example 5: Nested Groups (Complex)

```
Model:
  type team
    relations
      define member: [user, team#member]  # teams can contain other teams

  type document
    relations
      define viewer: [team#member]

Existing tuples:
  - team:platform#member@team:backend#member   # platform includes backend
  - team:backend#member@user:alice
  - document:doc1#viewer@team:platform#member

Tuple Write: team:backend#member@user:bob

Pre-computation:
  1. Classify: UsersetAssignment
  2. Find transitive group memberships:
     - bob is member of backend
     - backend is member of platform
     - So bob is transitively member of platform
  3. Find documents with viewer = team:platform#member or team:backend#member
     → [document:doc1]
  4. Affected checks:
     - (team:backend, member, user:bob)
     - (team:platform, member, user:bob)      # transitive
     - (document:doc1, viewer, user:bob)      # through platform
  5. Compute all checks

Result: 3 checks (fan-out through group hierarchy)
```

---

## 5. Optimization Strategies

### 5.1 Batching

```rust
impl PreComputeEngine {
    /// Process multiple changes together for efficiency
    pub async fn process_batch(&self, changes: Vec<TupleChange>) -> Result<(), Error> {
        // Group by store_id
        let by_store: HashMap<String, Vec<TupleChange>> = group_by_store(changes);

        for (store_id, store_changes) in by_store {
            // Collect all affected checks (deduplicated)
            let mut all_affected = HashSet::new();

            for change in &store_changes {
                let affected = self.find_affected_checks(change).await?;
                all_affected.extend(affected);
            }

            // Batch compute (can parallelize)
            let results = self.batch_compute(all_affected).await?;

            // Single publish per store
            self.publisher.publish(CheckResultSync {
                store_id,
                updates: results,
                watermark: store_changes.last().unwrap().ulid.clone(),
            }).await?;
        }

        Ok(())
    }
}
```

### 5.2 Incremental Computation

Instead of recomputing from scratch, track which parts of the graph changed:

```rust
/// Only recompute if the result might have changed
fn needs_recomputation(&self, key: &CheckKey, change: &TupleChange) -> bool {
    // If it was already allowed and we're adding, might not change
    // If it was denied and we're removing, might not change
    // Use this to skip unnecessary recomputations

    let cached_result = self.cache.get(key);
    match (cached_result, change.operation) {
        (Some(true), Operation::Write) => false,  // Already allowed, adding won't change
        (Some(false), Operation::Delete) => false, // Already denied, removing won't change
        _ => true,
    }
}
```

### 5.3 Bounded Fan-Out

For changes that affect many checks, limit the immediate computation:

```rust
const MAX_IMMEDIATE_FANOUT: usize = 1000;

async fn find_affected_checks_bounded(&self, change: &TupleChange) -> AffectedResult {
    let affected = self.find_affected_checks(change).await?;

    if affected.len() <= MAX_IMMEDIATE_FANOUT {
        AffectedResult::Complete(affected)
    } else {
        // Too many - compute high-priority ones immediately, queue the rest
        let (immediate, deferred) = split_by_priority(affected, MAX_IMMEDIATE_FANOUT);
        self.deferred_queue.enqueue(deferred).await?;
        AffectedResult::Partial(immediate)
    }
}
```

### 5.4 Application-Specific Filtering

Only compute checks that the application actually uses:

```rust
/// Edge configuration specifies which checks it cares about
pub struct EdgeSubscription {
    store_id: String,
    object_types: Option<Vec<String>>,    // None = all
    relations: Option<Vec<String>>,        // None = all
    user_patterns: Option<Vec<String>>,    // None = all
}

impl PreComputeEngine {
    fn should_publish_to_edge(&self, key: &CheckKey, edge: &EdgeSubscription) -> bool {
        // Filter by object type
        if let Some(types) = &edge.object_types {
            if !types.iter().any(|t| key.object.starts_with(t)) {
                return false;
            }
        }

        // Filter by relation
        if let Some(relations) = &edge.relations {
            if !relations.contains(&key.relation) {
                return false;
            }
        }

        // Filter by user pattern
        if let Some(patterns) = &edge.user_patterns {
            if !patterns.iter().any(|p| key.user.matches(p)) {
                return false;
            }
        }

        true
    }
}
```

---

## 6. Complexity Analysis

| Change Type | Affected Checks | Complexity |
|-------------|-----------------|------------|
| Direct assignment | 1 + computed relations | O(R) where R = relations in model |
| Computed userset | 1 + computed chain | O(R) |
| Userset (group) | 1 + objects referencing group | O(D) where D = documents with group |
| TTU (inheritance) | 1 + children + users | O(C × U) where C = children, U = users |
| Nested groups | Transitive closure | O(G × D) where G = group depth |

### Worst Case Scenarios

```
Scenario: Large team added as viewer to many documents

Existing:
  - 100,000 documents with viewer = team:all-employees#member
  - team:all-employees has 10,000 members

Change: team:all-employees#member@user:new-hire

Affected: 100,001 checks (1 team + 100,000 documents)

Mitigation:
  1. Process in batches (1000 at a time)
  2. Use deferred queue for large fan-outs
  3. Pre-compute during off-peak hours
  4. Consider materialized views for stable large groups
```

---

## 7. Edge Sync Protocol

```
┌─────────────────────────────────────────────────────────────────────────────────────┐
│                              SYNC MESSAGE FORMAT                                    │
│                                                                                     │
│  message CheckResultSync {                                                         │
│      string store_id = 1;                                                          │
│      repeated CheckResultUpdate updates = 2;                                       │
│      string watermark = 3;  // ULID for ordering                                   │
│  }                                                                                 │
│                                                                                     │
│  message CheckResultUpdate {                                                       │
│      uint64 key_hash = 1;           // Pre-computed hash for O(1) lookup          │
│      string object = 2;              // For debugging                              │
│      string relation = 3;                                                          │
│      string user = 4;                                                              │
│      bool allowed = 5;                                                             │
│      bool deleted = 6;               // Remove from cache                          │
│      uint64 computed_at = 7;         // Timestamp                                  │
│  }                                                                                 │
│                                                                                     │
│  Example message:                                                                  │
│  {                                                                                 │
│    "store_id": "store_app_a",                                                     │
│    "updates": [                                                                    │
│      {"key_hash": 12345, "object": "doc:1", "relation": "viewer",                 │
│       "user": "user:alice", "allowed": true},                                     │
│      {"key_hash": 12346, "object": "doc:1", "relation": "editor",                 │
│       "user": "user:alice", "allowed": true},                                     │
│      {"key_hash": 12347, "object": "doc:2", "relation": "viewer",                 │
│       "user": "user:alice", "allowed": true}                                      │
│    ],                                                                              │
│    "watermark": "01ARZ3NDEKTSV4RRFFQ69G5FAV"                                      │
│  }                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────────┘
```

---

## 8. Summary

### Pre-Computation Flow

```
1. Tuple written to Central DB
         │
         ▼
2. CDC captures change → Kafka
         │
         ▼
3. Pre-compute Worker consumes
         │
         ├─ Classify change type
         ├─ Find affected checks (reverse/forward expansion)
         ├─ Compute each check result
         │
         ▼
4. Publish to Kafka topic (partitioned by store_id)
         │
         ▼
5. Edge sidecars consume
         │
         ├─ Filter by subscription
         ├─ Update local HashMap
         │
         ▼
6. Check requests served from HashMap → <1ms
```

### Key Points

1. **Async Pre-computation**: Expensive graph traversal happens at central, not at edge
2. **Incremental Updates**: Only affected checks are recomputed
3. **Application Filtering**: Edges only receive relevant updates
4. **Bounded Fan-out**: Large changes are batched/deferred
5. **O(1) at Edge**: Pre-computed hash enables instant lookup

