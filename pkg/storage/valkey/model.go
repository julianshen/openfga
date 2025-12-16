package valkey

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/oklog/ulid/v2"
	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/encoding/protojson"

	openfgav1 "github.com/openfga/api/proto/openfga/v1"

	"github.com/openfga/openfga/pkg/storage"
)

const (
	modelsIndexPrefix = "models:index"
)

func modelsIndexKey(storeID string) string {
	return fmt.Sprintf("%s:%s", modelsIndexPrefix, storeID)
}

func (s *ValkeyBackend) ReadAuthorizationModel(ctx context.Context, store string, id string) (*openfgav1.AuthorizationModel, error) {
	ctx, span := tracer.Start(ctx, "valkey.ReadAuthorizationModel")
	defer span.End()

	// If ID is empty, find latest
	if id == "" {
		return s.FindLatestAuthorizationModel(ctx, store)
	}

	val, err := s.client.Get(ctx, authorizationModelKey(store, id)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}

	var model openfgav1.AuthorizationModel
	if err := protojson.Unmarshal(val, &model); err != nil {
		return nil, err
	}

	return &model, nil
}

func (s *ValkeyBackend) ReadAuthorizationModels(ctx context.Context, store string, options storage.ReadAuthorizationModelsOptions) ([]*openfgav1.AuthorizationModel, string, error) {
	ctx, span := tracer.Start(ctx, "valkey.ReadAuthorizationModels")
	defer span.End()

	// Pagination Logic
	var currentCursor *zsetCursor
	useRank := false
	offset := int64(0)

	count := int64(storage.DefaultPageSize)
	if options.Pagination.PageSize > 0 {
		count = int64(options.Pagination.PageSize)
	}

	if options.Pagination.From != "" {
		if c, err := decodeZSetCursor(options.Pagination.From); err == nil {
			currentCursor = c
		} else {
			// Fallback to offset (legacy behavior)
			if parsed, err := strconv.ParseInt(options.Pagination.From, 10, 64); err == nil {
				useRank = true
				offset = parsed
			} else {
				return nil, "", storage.ErrInvalidContinuationToken
			}
		}
	}

	var ids []string
	var err error

	// Query
	if useRank {
		// ZREVRANGE key start stop (Rank based)
		ids, err = s.client.ZRevRange(ctx, modelsIndexKey(store), offset, offset+count-1).Result()
	} else {
		// CURSOR based (Optimized)
		// We want Descending order (Rev).
		// Max score is determined by cursor (inclusive start point downwards).
		max := "+inf"
		if currentCursor != nil {
			max = strconv.FormatFloat(currentCursor.Score, 'f', -1, 64)
		}

		// Fetch extra items to handle ties at score boundaries
		fetchCount := count + 5

		// ZRevRangeByScore logic using ZRangeArgs
		var zIds []redis.Z
		zIds, err = s.client.ZRangeArgsWithScores(ctx, redis.ZRangeArgs{
			Key:     modelsIndexKey(store),
			Start:   max,    // Max score (start of reverse scan)
			Stop:    "-inf", // Min score
			ByScore: true,
			Rev:     true,
			Count:   fetchCount,
		}).Result()
		if err != nil {
			return nil, "", err
		}

		// Filter out already-seen items when resuming from cursor
		scanCount := 0
		for _, z := range zIds {
			member, _ := z.Member.(string)
			score := z.Score

			if currentCursor != nil {
				// In reverse scan, Redis sorts ties by Member DESC.
				// If we saw Member "B" at score X, next should be "A" or lower score.
				// Skip if score == cursor.Score AND member >= cursor.Member
				if score == currentCursor.Score && member >= currentCursor.Member {
					continue // Already seen
				}
			}

			ids = append(ids, member)
			scanCount++
			if scanCount >= int(count) {
				break
			}
		}
	}

	if err != nil {
		return nil, "", err
	}

	if len(ids) == 0 {
		return nil, "", nil
	}

	var modelKeys []string
	for _, id := range ids {
		modelKeys = append(modelKeys, authorizationModelKey(store, id))
	}

	vals, err := s.client.MGet(ctx, modelKeys...).Result()
	if err != nil {
		return nil, "", err
	}

	var models []*openfgav1.AuthorizationModel
	for _, val := range vals {
		if val == nil {
			continue
		}
		sStr, ok := val.(string)
		if !ok {
			continue
		}
		var m openfgav1.AuthorizationModel
		if err := protojson.Unmarshal([]byte(sStr), &m); err != nil {
			continue
		}
		models = append(models, &m)
	}

	contToken := ""
	// Generate Token from last item
	if len(models) == int(count) {
		lastModel := models[len(models)-1]
		// Calculate score (ms timestamp from ID)
		ulidID, _ := ulid.Parse(lastModel.GetId())
		contToken = encodeZSetCursor(float64(ulidID.Time()), lastModel.GetId())
	}

	return models, contToken, nil
}

func (s *ValkeyBackend) FindLatestAuthorizationModel(ctx context.Context, store string) (*openfgav1.AuthorizationModel, error) {
	ctx, span := tracer.Start(ctx, "valkey.FindLatestAuthorizationModel")
	defer span.End()

	id, err := s.client.Get(ctx, latestAuthorizationModelKey(store)).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}

	return s.ReadAuthorizationModel(ctx, store, id)
}

func (s *ValkeyBackend) WriteAuthorizationModel(ctx context.Context, store string, model *openfgav1.AuthorizationModel) error {
	ctx, span := tracer.Start(ctx, "valkey.WriteAuthorizationModel")
	defer span.End()

	id := model.GetId()
	if id == "" {
		return errors.New("model ID required") // Should be generated by caller usually? No, OpenFGA caller generates it.
	}

	bytes, err := protojson.Marshal(model)
	if err != nil {
		return err
	}

	// Calculate score from ULID
	ulidID, err := ulid.Parse(id)
	if err != nil {
		return err
	}
	score := float64(ulidID.Time())

	pipeline := s.client.TxPipeline()
	pipeline.Set(ctx, authorizationModelKey(store, id), bytes, 0)
	pipeline.Set(ctx, latestAuthorizationModelKey(store), id, 0)
	pipeline.ZAdd(ctx, modelsIndexKey(store), redis.Z{
		Score:  score,
		Member: id,
	})

	_, err = pipeline.Exec(ctx)
	return err
}
