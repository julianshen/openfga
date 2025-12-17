package valkey

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestZSetCursor(t *testing.T) {
	t.Run("round_trip", func(t *testing.T) {
		score := float64(time.Now().UnixNano())
		member := "store-123"

		token := encodeZSetCursor(score, member)
		require.NotEmpty(t, token)

		cursor, err := decodeZSetCursor(token)
		require.NoError(t, err)
		require.NotNil(t, cursor)
		require.InEpsilon(t, score, cursor.Score, 0.0000001)
		require.Equal(t, member, cursor.Member)
	})

	t.Run("invalid_token", func(t *testing.T) {
		cursor, err := decodeZSetCursor("invalid-base64")
		require.Error(t, err)
		require.Nil(t, cursor)
	})

	t.Run("invalid_json", func(t *testing.T) {
		// Encoded valid base64 but invalid JSON
		// "not-json" -> base64 -> "bm90LWpzb24="
		token := "bm90LWpzb24="
		cursor, err := decodeZSetCursor(token)
		require.Error(t, err)
		require.Nil(t, cursor)
	})
}
