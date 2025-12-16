package valkey_test

import (
	"context"
	"testing"
	"time"

	"github.com/openfga/openfga/pkg/storage/test"
	"github.com/openfga/openfga/pkg/storage/valkey"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestValkeyDatastore(t *testing.T) {
	uri := "redis://localhost:6380"

	ds, err := valkey.New(uri)
	require.NoError(t, err)
	// Check readiness
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	status, err := ds.IsReady(ctx)
	if err != nil {
		t.Skipf("Valkey not ready at %s: %v", uri, err)
	}
	if !status.IsReady {
		t.Skipf("Valkey not ready at %s", uri)
	}

	// Flush DB before running tests
	client := redis.NewClient(&redis.Options{Addr: "localhost:6380"})
	require.NoError(t, client.FlushDB(context.Background()).Err())
	client.Close()

	test.RunAllTests(t, ds)
}
