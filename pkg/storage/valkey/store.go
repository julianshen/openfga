package valkey

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	openfgav1 "github.com/openfga/api/proto/openfga/v1"
	"github.com/openfga/openfga/pkg/storage"
)

const (
	storesIndexKey     = "stores:index"
	storesNameIndexKey = "stores:name_index"
)

func (s *ValkeyBackend) CreateStore(ctx context.Context, store *openfgav1.Store) (*openfgav1.Store, error) {
	ctx, span := tracer.Start(ctx, "valkey.CreateStore")
	defer span.End()

	if store.GetId() == "" || store.GetName() == "" {
		return nil, errors.New("store id and name are required")
	}

	// Prepare data
	now := timestamppb.New(time.Now().UTC())
	store.CreatedAt = now
	store.UpdatedAt = now

	bytes, err := protojson.Marshal(store)
	if err != nil {
		return nil, err
	}

	// Transaction
	// pipe was unused

	// Check if ID exists (Watcher) - actually we can use NX options, but we need to check Name uniqueness too.
	// Optimistic locking? Or just use SETNX.
	// Name uniqueness is global.

	// Check if name exists
	// We can't easily check and set in pipeline with logic conditions.
	// We should use Watch.

	err = s.client.Watch(ctx, func(tx *redis.Tx) error {
		// Check ID conflict
		exists, err := tx.Exists(ctx, storeKey(store.GetId())).Result()
		if err != nil {
			return err
		}
		if exists > 0 {
			return storage.ErrCollision // ID collision
		}

		// Verify duplicate name check is NOT required by interface (Storage Test allows duplicates)
		// We just index it.

		_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.Set(ctx, storeKey(store.GetId()), bytes, 0)
			// Add to Name Index (ZSet for ordering)
			pipe.ZAdd(ctx, storesByNameKey(store.GetName()), redis.Z{
				Score:  float64(now.AsTime().UnixNano()),
				Member: store.GetId(),
			})
			pipe.ZAdd(ctx, storesIndexKey, redis.Z{
				Score:  float64(now.AsTime().UnixNano()),
				Member: store.GetId(),
			})
			return nil
		})
		return err
	}, storeKey(store.GetId()))

	if err != nil {
		if err == redis.TxFailedErr {
			// Collision during watch (specifically ID collision likely)
			return nil, storage.ErrCollision
		}
		return nil, err
	}

	return store, nil
}

func (s *ValkeyBackend) DeleteStore(ctx context.Context, id string) error {
	ctx, span := tracer.Start(ctx, "valkey.DeleteStore")
	defer span.End()

	// Get store to find name
	storeBytes, err := s.client.Get(ctx, storeKey(id)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil
		}
		return err
	}

	var store openfgav1.Store
	if err := protojson.Unmarshal(storeBytes, &store); err != nil {
		return err
	}

	pipeline := s.client.TxPipeline()
	pipeline.Del(ctx, storeKey(id))
	// Remove from Name Index
	pipeline.ZRem(ctx, storesByNameKey(store.GetName()), id)
	pipeline.ZRem(ctx, storesIndexKey, id)
	_, err = pipeline.Exec(ctx)
	return err
}

func (s *ValkeyBackend) GetStore(ctx context.Context, id string) (*openfgav1.Store, error) {
	ctx, span := tracer.Start(ctx, "valkey.GetStore")
	defer span.End()

	val, err := s.client.Get(ctx, storeKey(id)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}

	var store openfgav1.Store
	if err := protojson.Unmarshal(val, &store); err != nil {
		return nil, err
	}

	return &store, nil
}

func (s *ValkeyBackend) ListStores(ctx context.Context, options storage.ListStoresOptions) ([]*openfgav1.Store, string, error) {
	ctx, span := tracer.Start(ctx, "valkey.ListStores")
	defer span.End()

	var stores []*openfgav1.Store

	// Case 1: Filter by IDs
	if len(options.IDs) > 0 {
		var keys []string
		for _, id := range options.IDs {
			keys = append(keys, storeKey(id))
		}
		if len(keys) == 0 {
			return nil, "", nil
		}

		vals, err := s.client.MGet(ctx, keys...).Result()
		if err != nil {
			return nil, "", err
		}

		for _, val := range vals {
			if val == nil {
				continue
			}
			sStr, ok := val.(string)
			if !ok {
				continue
			}

			var store openfgav1.Store
			if err := protojson.Unmarshal([]byte(sStr), &store); err != nil {
				return nil, "", err
			}

			if options.Name != "" && store.GetName() != options.Name {
				continue
			}
			stores = append(stores, &store)
		}
		return stores, "", nil
	}

	offset := int64(0)
	if options.Pagination.From != "" {
		fmt.Sscanf(options.Pagination.From, "%d", &offset)
	}
	count := int64(storage.DefaultPageSize)
	if options.Pagination.PageSize > 0 {
		count = int64(options.Pagination.PageSize)
	}

	var ids []string
	var err error

	// Case 2: Filter by Name
	if options.Name != "" {
		// ZRANGE on Name Index
		ids, err = s.client.ZRange(ctx, storesByNameKey(options.Name), offset, offset+count-1).Result()
	} else {
		// Case 3: List all with Pagination (ZRANGE)
		// We want Ascending order by creation time? Memory backend does slice[start:start+limit] which is insertion order.
		// So ZRange (Ascending)
		ids, err = s.client.ZRange(ctx, storesIndexKey, offset, offset+count-1).Result()
	}

	if err != nil {
		return nil, "", err
	}

	// Now fetch the stores
	if len(ids) > 0 {
		var keys []string
		for _, id := range ids {
			keys = append(keys, storeKey(id))
		}
		vals, err := s.client.MGet(ctx, keys...).Result()
		if err != nil {
			return nil, "", err
		}
		for _, val := range vals {
			if val == nil {
				continue
			}
			sStr, ok := val.(string)
			if !ok {
				continue
			}
			var st openfgav1.Store
			if err := protojson.Unmarshal([]byte(sStr), &st); err != nil {
				continue
			}
			stores = append(stores, &st)
		}
	}

	contToken := ""
	if len(ids) == int(count) {
		contToken = fmt.Sprintf("%d", offset+count)
	}
	return stores, contToken, nil
}
