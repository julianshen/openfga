package valkey

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	openfgav1 "github.com/openfga/api/proto/openfga/v1"

	"github.com/openfga/openfga/pkg/storage"
)

type ChangelogEntry struct {
	TupleKey  *openfgav1.TupleKey      `json:"tk,omitempty"`
	Operation openfgav1.TupleOperation `json:"op,omitempty"`
	Timestamp time.Time                `json:"ts,omitempty"`
}

func (s *ValkeyBackend) LogChange(ctx context.Context, pipe redis.Pipeliner, store string, change *openfgav1.TupleChange) error {
	// We use Redis native IDs (timestamp-sequence) for changelog entries to ensure
	// strict ordering and atomicity without coordination.
	//
	// The OpenFGADatastore interface uses string continuation tokens. existing tests/implementations
	// often expect ULIDs.
	// - On Write (LogChange): We let Redis generate the ID and do not enforce ULID.
	// - On Read (ReadChanges):
	//   - We return the Redis ID as the continuation token.
	//   - If the input token is a Redis ID, we use it directly for resumption (exclusive range).
	//   - If the input token is a ULID (e.g. from a different context), we make a best-effort
	//     approximation by using the ULID timestamp to find the closest Redis ID, acknowledging
	//     that intra-millisecond precision relative to the ULID is lost.

	bytes, err := protojson.Marshal(change.GetTupleKey())
	if err != nil {
		return err
	}

	// XADD changelog:{store} * tk ... op ...
	// We store minimal data.
	values := map[string]interface{}{
		"tk": string(bytes),
		"op": int(change.GetOperation()),
		// Store timestamp explicitly too, in case we need it?
		// No, we can rely on Redis ID time, OR we should store the change timestamp if it was passed in.
		// change.Timestamp
	}

	pipe.XAdd(ctx, &redis.XAddArgs{
		Stream: changelogKey(store),
		Values: values,
	})
	return nil
}

func (s *ValkeyBackend) ReadChanges(ctx context.Context, store string, filter storage.ReadChangesFilter, options storage.ReadChangesOptions) ([]*openfgav1.TupleChange, string, error) {
	ctx, span := tracer.Start(ctx, "valkey.ReadChanges")
	defer span.End()

	count := int64(storage.DefaultPageSize)
	if options.Pagination.PageSize > 0 {
		count = int64(options.Pagination.PageSize)
	}

	start := "-"
	if options.Pagination.From != "" {
		// Try to parse as ULID first
		u, err := ulid.Parse(options.Pagination.From)
		if err == nil {
			// It is a ULID. Use its timestamp.
			// Format for Redis: <ms>-<seq>.
			// We can use <ms>-0 to start from that time.
			// Wait, if it's a continuation token of a specific event, we want to start AFTER it.
			// Redis `(` prefix means exclusive.
			// But ULID random part is not mapped to Redis Seq.
			// So we might get duplicates if multiple events happened at the same ms?
			// This is a limitation if we don't store the ULID as the ID.
			// If we just use the timestamp:
			start = strconv.FormatUint(u.Time(), 10)
		} else {
			// Try as Redis ID
			start = options.Pagination.From
			// If we want to start AFTER the token:
			// "The token ... can be used to retrieve the next page"
			// Usually token points to the LAST item of previous page.
			// So we want > token.
			// Redis XREAD or XRANGE with `(`
			start = "(" + start
		}
	} else if options.SortDesc {
		start = "+" // Reverse iteration, start from end
	}

	// Horizon offset
	// Filter out changes that are "too new"?
	// "Changes that occur after this offset will not be included"
	// change.Timestamp > now - offset => Exclude.
	// change.Timestamp <= now - offset => Include.
	// So we want to read up to `now - offset`.
	// end = now - offset.

	if options.SortDesc {
		// Reverse range: XRANGE key end start
		// But Wait, Redis terminology: XRANGE key start end.
		// REV: XREVRANGE key end start [COUNT count]
		// end is the "higher" ID (max), start is "lower" ID (min).
		// So if SortDesc:
		// We want FROM "latest" (or provided token) DOWN TO "oldest".
		// If provided token `start` (from From) is provided, that's our upper bound (exclusive).
		// If not provided, upper bound is `end` (calculated by horizon).

		// Let's refine.
		// Start ID (highest): If From is set, use it (exclusive). If not, use Horizon limit.
		maxID := "+"
		if options.Pagination.From != "" {
			maxID = "(" + options.Pagination.From
		} else if filter.HorizonOffset > 0 {
			cutoff := time.Now().Add(-filter.HorizonOffset)
			maxID = strconv.FormatInt(cutoff.UnixMilli(), 10)
		}

		minID := "-"

		cmd := s.client.XRevRangeN(ctx, changelogKey(store), maxID, minID, count)
		return s.processStreamResults(cmd, filter)
	} else {
		// Forward range
		// Start ID (lowest): If From is set, used it (exclusive). Else "-".
		// End ID (highest): Horizon limit.

		minID := "-" // Default
		if options.Pagination.From != "" {
			// If From was parsed as timestamp-only (ULID), we just use it.
			// If it was Redis ID, we use exclusive.
			// My logic above for `start` handled this.
			minID = start
		}

		maxID := "+"
		if filter.HorizonOffset > 0 {
			cutoff := time.Now().Add(-filter.HorizonOffset)
			maxID = strconv.FormatInt(cutoff.UnixMilli(), 10)
		}

		cmd := s.client.XRangeN(ctx, changelogKey(store), minID, maxID, count)
		return s.processStreamResults(cmd, filter)
	}
}

func (s *ValkeyBackend) processStreamResults(cmd *redis.XMessageSliceCmd, filter storage.ReadChangesFilter) ([]*openfgav1.TupleChange, string, error) {
	msgs, err := cmd.Result()
	if err != nil {
		return nil, "", err
	}

	if len(msgs) == 0 {
		return nil, "", nil
	}

	var changes []*openfgav1.TupleChange
	lastID := ""

	for _, msg := range msgs {
		lastID = msg.ID
		// Parse
		tkStr, ok := msg.Values["tk"].(string)
		if !ok {
			continue
		}

		var tk openfgav1.TupleKey
		if err := protojson.Unmarshal([]byte(tkStr), &tk); err != nil {
			continue
		}

		if tk.GetCondition() != nil && tk.GetCondition().GetContext() == nil {
			tk.GetCondition().Context = &structpb.Struct{}
		}

		// Filter by ObjectType
		if filter.ObjectType != "" && !strings.HasPrefix(tk.GetObject(), filter.ObjectType+":") {
			continue
		}

		opInt := 0
		// Redis streams store values as strings
		if opS, ok := msg.Values["op"].(string); ok {
			if parsed, err := strconv.Atoi(opS); err == nil {
				opInt = parsed
			}
		} else if opN, ok := msg.Values["op"].(int64); ok {
			opInt = int(opN)
		}

		// Reconstruct TupleChange
		// Timestamp from Msg ID
		tsParts := strings.Split(msg.ID, "-")
		if len(tsParts) == 0 {
			continue
		}
		tsMs, err := strconv.ParseInt(tsParts[0], 10, 64)
		if err != nil {
			continue
		}
		ts := time.UnixMilli(tsMs)

		changes = append(changes, &openfgav1.TupleChange{
			TupleKey:  &tk,
			Operation: openfgav1.TupleOperation(opInt),
			Timestamp: timestamppb.New(ts),
		})
	}

	return changes, lastID, nil
}
