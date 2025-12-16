package valkey

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	openfgav1 "github.com/openfga/api/proto/openfga/v1"
	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/encoding/protojson"
)

func (s *ValkeyBackend) WriteAssertions(ctx context.Context, store, modelID string, assertions []*openfgav1.Assertion) error {
	ctx, span := tracer.Start(ctx, "valkey.WriteAssertions")
	defer span.End()

	// Serialize manually a list of proto messages
	var jsonStrings []string
	for _, assertion := range assertions {
		bytes, err := protojson.Marshal(assertion)
		if err != nil {
			return err
		}
		jsonStrings = append(jsonStrings, string(bytes))
	}

	val := fmt.Sprintf("[%s]", strings.Join(jsonStrings, ","))

	return s.client.Set(ctx, assertionsKey(store, modelID), val, 0).Err()
}

func (s *ValkeyBackend) ReadAssertions(ctx context.Context, store, modelID string) ([]*openfgav1.Assertion, error) {
	ctx, span := tracer.Start(ctx, "valkey.ReadAssertions")
	defer span.End()

	val, err := s.client.Get(ctx, assertionsKey(store, modelID)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return []*openfgav1.Assertion{}, nil
		}
		return nil, err
	}

	// Unwrap JSON array.
	// Standard JSON unmarshal into []json.RawMessage
	var rawMessages []json.RawMessage
	if err := json.Unmarshal(val, &rawMessages); err != nil {
		return nil, err
	}

	assertions := make([]*openfgav1.Assertion, len(rawMessages))
	for i, raw := range rawMessages {
		var a openfgav1.Assertion
		if err := protojson.Unmarshal(raw, &a); err != nil {
			return nil, err
		}
		assertions[i] = &a
	}

	return assertions, nil
}
