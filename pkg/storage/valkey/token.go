package valkey

import (
	"encoding/base64"

	jsoniter "github.com/json-iterator/go"
)

var jsonIter = jsoniter.ConfigCompatibleWithStandardLibrary

type zsetCursor struct {
	Score  float64 `json:"s"`
	Member string  `json:"m"`
}

func encodeZSetCursor(score float64, member string) string {
	cursor := zsetCursor{
		Score:  score,
		Member: member,
	}
	bytes, err := jsonIter.Marshal(cursor)
	if err != nil {
		// Should not happen for defined struct
		return ""
	}
	// Use RawStdEncoding to avoid padding
	return base64.RawStdEncoding.EncodeToString(bytes)
}

func decodeZSetCursor(token string) (*zsetCursor, error) {
	bytes, err := base64.RawStdEncoding.DecodeString(token)
	if err != nil {
		return nil, err
	}
	var cursor zsetCursor
	if err := jsonIter.Unmarshal(bytes, &cursor); err != nil {
		return nil, err
	}
	return &cursor, nil
}
