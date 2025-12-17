package valkey

import (
	"context"
	"strings"
	"sync"

	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"

	openfgav1 "github.com/openfga/api/proto/openfga/v1"

	"github.com/openfga/openfga/pkg/storage"
	tupleUtils "github.com/openfga/openfga/pkg/tuple"
)

type valkeyTupleIterator struct {
	client *redis.Client
	store  string
	// Base filter info
	object   string
	relation string
	user     string // For reverse

	// Index Key to scan
	indexKey string
	// Match pattern for Full Scan
	matchPattern string

	// Scan state
	cursor uint64
	buffer []*openfgav1.Tuple
	eof    bool

	// Iterator type specific logic
	// 0: Normal (Obj/Rel -> scan users)
	// 1: Reverse (User -> scan Obj/Rel)
	// 2: Userset (Obj/Rel -> scan users -> filter userset)
	// 3: Full Scan (Scan keyspace)
	iterType int

	// Filters for Userset/Reverse
	allowedTypes   []*openfgav1.RelationReference
	targetObjType  string
	targetRelation string

	mu sync.Mutex
}

const (
	iterTypeNormal   = 0
	iterTypeReverse  = 1
	iterTypeUserset  = 2
	iterTypeFullScan = 3

	scanBatchSize = 100 // default scan count
)

// NewTupleIterator scans users from index:obj_rel.
func NewTupleIterator(ctx context.Context, _ *redis.ScanIterator, store, object, relation, userFilter string, client *redis.Client) storage.TupleIterator {
	return &valkeyTupleIterator{
		client:   client,
		store:    store,
		object:   object,
		relation: relation,
		indexKey: indexObjectRelationKey(store, object, relation),
		iterType: iterTypeNormal,
	}
}

// NewReverseTupleIterator scans obj#rel from index:user.
func NewReverseTupleIterator(ctx context.Context, _ *redis.ScanIterator, store, user, objectType, relation string, client *redis.Client) storage.TupleIterator {
	return &valkeyTupleIterator{
		client:         client,
		store:          store,
		user:           user,
		indexKey:       indexUserKey(store, user),
		iterType:       iterTypeReverse,
		targetObjType:  objectType,
		targetRelation: relation,
	}
}

// NewUsersetTupleIterator scans users from index:obj_rel and filters usersets.
func NewUsersetTupleIterator(ctx context.Context, _ *redis.ScanIterator, store, object, relation string, allowedTypes []*openfgav1.RelationReference, client *redis.Client) storage.TupleIterator {
	return &valkeyTupleIterator{
		client:       client,
		store:        store,
		object:       object,
		relation:     relation,
		indexKey:     indexObjectRelationKey(store, object, relation),
		iterType:     iterTypeUserset,
		allowedTypes: allowedTypes,
	}
}

// NewFullScanIterator scans keyspace for tuples.
func NewFullScanIterator(ctx context.Context, store string, client *redis.Client) storage.TupleIterator {
	return &valkeyTupleIterator{
		client:       client,
		store:        store,
		iterType:     iterTypeFullScan,
		matchPattern: tupleKey(store, "*", "*", "*"),
	}
}

func (i *valkeyTupleIterator) fetchBatch(ctx context.Context) error {
	for !i.eof && len(i.buffer) == 0 {
		var keys []string
		var nextCursor uint64
		var err error

		if i.iterType == iterTypeFullScan {
			keys, nextCursor, err = i.client.Scan(ctx, i.cursor, i.matchPattern, scanBatchSize).Result()
		} else {
			// Scan Set
			keys, nextCursor, err = i.client.SScan(ctx, i.indexKey, i.cursor, "", scanBatchSize).Result()
		}

		if err != nil {
			return err
		}

		i.cursor = nextCursor
		if i.cursor == 0 {
			i.eof = true
		}

		if len(keys) == 0 {
			continue
		}

		// Prepare keys to MGET
		var tupleKeys []string

		// Temporary storage to map back
		// Note: The index stores partial info.
		// iterTypeNormal/Userset: keys are Users.
		// iterTypeReverse: keys are Object#Relation.

		for _, k := range keys {
			switch i.iterType {
			case iterTypeFullScan:
				tupleKeys = append(tupleKeys, k)

			case iterTypeNormal:
				// k is User
				// If userFilter logic needed? (Read() usually handles exact user match separately)
				tupleKeys = append(tupleKeys, tupleKey(i.store, i.object, i.relation, k))

			case iterTypeUserset:
				// k is User
				// Filter for userset
				if tupleUtils.GetUserTypeFromUser(k) != tupleUtils.UserSet {
					continue
				}

				// Filter allowed types
				if len(i.allowedTypes) > 0 {
					userType := tupleUtils.GetType(k)
					_, userRelation := tupleUtils.SplitObjectRelation(k)
					allowed := false
					for _, at := range i.allowedTypes {
						if at.GetType() == userType && at.GetRelation() == userRelation {
							allowed = true
							break
						}
					}
					if !allowed {
						continue
					}
				}

				tupleKeys = append(tupleKeys, tupleKey(i.store, i.object, i.relation, k))

			case iterTypeReverse:
				// k is Object#Relation
				parts := strings.SplitN(k, "#", 2)
				if len(parts) != 2 {
					continue
				}
				obj, rel := parts[0], parts[1]

				if i.targetObjType != "" {
					td, _ := tupleUtils.SplitObject(obj)
					if td != i.targetObjType {
						continue
					}
				}
				if i.targetRelation != "" && rel != i.targetRelation {
					continue
				}

				tupleKeys = append(tupleKeys, tupleKey(i.store, obj, rel, i.user))
			}
		}

		if len(tupleKeys) == 0 {
			continue
		}

		// MGET
		vals, err := i.client.MGet(ctx, tupleKeys...).Result()
		if err != nil {
			return err
		}

		for _, val := range vals {
			if val == nil {
				continue
			}
			sStr, ok := val.(string)
			if !ok {
				continue
			}

			var t openfgav1.Tuple
			if err := protojson.Unmarshal([]byte(sStr), &t); err != nil {
				continue
			}
			if t.GetKey() != nil && t.GetKey().GetCondition() != nil {
				if t.GetKey().GetCondition().GetContext() == nil {
					t.GetKey().GetCondition().Context = &structpb.Struct{}
				}
			}
			i.buffer = append(i.buffer, &t)
		}
	}
	return nil
}

func (i *valkeyTupleIterator) Next(ctx context.Context) (*openfgav1.Tuple, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	if len(i.buffer) == 0 {
		if i.eof {
			return nil, storage.ErrIteratorDone
		}
		if err := i.fetchBatch(ctx); err != nil {
			return nil, err
		}
		if len(i.buffer) == 0 && i.eof {
			return nil, storage.ErrIteratorDone
		}
	}

	// Pop
	t := i.buffer[0]
	i.buffer = i.buffer[1:]
	return t, nil
}

func (i *valkeyTupleIterator) Head(ctx context.Context) (*openfgav1.Tuple, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	if len(i.buffer) == 0 {
		if i.eof {
			return nil, storage.ErrIteratorDone
		}
		if err := i.fetchBatch(ctx); err != nil {
			return nil, err
		}
		if len(i.buffer) == 0 && i.eof {
			return nil, storage.ErrIteratorDone
		}
	}

	return i.buffer[0], nil
}

func (i *valkeyTupleIterator) Stop() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.buffer = nil
	i.eof = true
}
