package valkey_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
	openfgav1 "github.com/openfga/api/proto/openfga/v1"
	"github.com/openfga/openfga/pkg/storage"
	"github.com/openfga/openfga/pkg/storage/valkey"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
)

func BenchmarkValkeyWrite(b *testing.B) {
	uri := os.Getenv("OPENFGA_VALKEY_URI")
	if uri == "" {
		uri = "redis://localhost:6380"
	}
	ds, err := valkey.New(uri)
	if err != nil {
		b.Skipf("Skipping benchmark: %v", err)
	}
	defer ds.Close()

	if _, err := ds.IsReady(context.Background()); err != nil {
		b.Skip("Valkey not ready")
	}

	ctx := context.Background()
	storeID := ulid.Make().String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tk := &openfgav1.TupleKey{
			Object:   fmt.Sprintf("doc:%d", i),
			Relation: "viewer",
			User:     "user:jon",
		}
		err := ds.Write(ctx, storeID, nil, []*openfgav1.TupleKey{tk})
		require.NoError(b, err)
	}
}

func BenchmarkValkeyReadUserTuple(b *testing.B) {
	uri := os.Getenv("OPENFGA_VALKEY_URI")
	if uri == "" {
		uri = "redis://localhost:6380"
	}
	ds, err := valkey.New(uri)
	if err != nil {
		b.Skipf("Skipping benchmark: %v", err)
	}
	defer ds.Close()

	if _, err := ds.IsReady(context.Background()); err != nil {
		b.Skip("Valkey not ready")
	}

	ctx := context.Background()
	storeID := ulid.Make().String()

	tk := &openfgav1.TupleKey{
		Object:   "doc:1",
		Relation: "viewer",
		User:     "user:jon",
	}
	require.NoError(b, ds.Write(ctx, storeID, nil, []*openfgav1.TupleKey{tk}))

	filter := storage.ReadUserTupleFilter{
		Object:   "doc:1",
		Relation: "viewer",
		User:     "user:jon",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ds.ReadUserTuple(ctx, storeID, filter, storage.ReadUserTupleOptions{})
		require.NoError(b, err)
	}
}

func BenchmarkListStores_DeepPagination(b *testing.B) {
	uri := os.Getenv("OPENFGA_VALKEY_URI")
	if uri == "" {
		uri = "redis://localhost:6380"
	}
	ds, err := valkey.New(uri)
	if err != nil {
		b.Skipf("Skipping benchmark: %v", err)
	}
	defer ds.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := ds.IsReady(ctx); err != nil {
		b.Skip("Valkey not ready")
	}

	// Setup: Clean and Seed 10k stores
	opt, err := redis.ParseURL(uri)
	require.NoError(b, err)
	client := redis.NewClient(opt)
	defer client.Close()
	require.NoError(b, client.FlushDB(context.Background()).Err())

	// Batch create stores using pipeline for speed
	// We assume CreateStore implementation details here for speed.
	// keys: store:{id}, stores:index, stores:by_name:{name}
	pipe := client.Pipeline()
	now := float64(time.Now().UnixNano()) // Simplified timestamp
	for i := 0; i < 10000; i++ {
		id := ulid.Make().String()
		name := fmt.Sprintf("store-%d", i)
		store := &openfgav1.Store{
			Id:        id,
			Name:      name,
			CreatedAt: nil, // simplified
			UpdatedAt: nil,
		}
		bytes, _ := protojson.Marshal(store)

		pipe.Set(context.Background(), fmt.Sprintf("store:%s", id), bytes, 0)
		pipe.ZAdd(context.Background(), "stores:index", redis.Z{Score: now, Member: id})
		pipe.ZAdd(context.Background(), fmt.Sprintf("stores:by_name:%s", name), redis.Z{Score: now, Member: id})

		if i%1000 == 0 {
			_, err := pipe.Exec(context.Background())
			require.NoError(b, err)
			pipe = client.Pipeline()
		}
	}
	_, err = pipe.Exec(context.Background())
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulations: Read page size 20 at offset 9900
		opts := storage.ListStoresOptions{
			Pagination: storage.PaginationOptions{
				PageSize: 20,
				From:     "9900",
			},
		}
		_, _, err := ds.ListStores(ctx, opts)
		require.NoError(b, err)
	}
}
