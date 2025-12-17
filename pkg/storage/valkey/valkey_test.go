package valkey_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/openfga/openfga/pkg/storage/test"
	"github.com/openfga/openfga/pkg/storage/valkey"
)

func TestValkeyDatastore(t *testing.T) {
	uri := os.Getenv("OPENFGA_VALKEY_URI")
	if uri == "" {
		uri = "redis://localhost:6380"
	}

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
	// Parse URI to extract address
	opt, err := redis.ParseURL(uri)
	require.NoError(t, err)
	client := redis.NewClient(opt)
	require.NoError(t, client.FlushDB(context.Background()).Err())
	client.Close()

	test.RunAllTests(t, ds)
}
