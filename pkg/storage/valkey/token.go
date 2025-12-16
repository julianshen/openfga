package valkey

import (
	"encoding/base64"
	"encoding/json"
)

type zsetCursor struct {
	Score  float64 `json:"s"`
	Member string  `json:"m"`
}

func encodeZSetCursor(score float64, member string) string {
	cursor := zsetCursor{
		Score:  score,
		Member: member,
	}
	bytes, err := json.Marshal(cursor)
	if err != nil {
		// Should not happen for defined struct
		return ""
	}
	return base64.StdEncoding.EncodeToString(bytes)
}

func decodeZSetCursor(token string) (*zsetCursor, error) {
	bytes, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return nil, err
	}
	var cursor zsetCursor
	if err := json.Unmarshal(bytes, &cursor); err != nil {
		return nil, err
	}
	return &cursor, nil
}
