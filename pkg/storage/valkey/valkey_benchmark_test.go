package valkey_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/oklog/ulid/v2"
	openfgav1 "github.com/openfga/api/proto/openfga/v1"
	"github.com/openfga/openfga/pkg/storage"
	"github.com/openfga/openfga/pkg/storage/valkey"
	"github.com/stretchr/testify/require"
)

func BenchmarkValkeyWrite(b *testing.B) {
	uri := "redis://localhost:6380"
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
	uri := "redis://localhost:6380"
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
