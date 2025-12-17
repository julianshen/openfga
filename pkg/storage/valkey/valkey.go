package valkey

import (
	"context"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"

	"github.com/openfga/openfga/pkg/storage"
)

var tracer = otel.Tracer("openfga/pkg/storage/valkey")

// ValkeyOption defines a function type used for configuring a [ValkeyBackend] instance.
type ValkeyOption func(dataStore *ValkeyBackend)

type ValkeyBackend struct {
	client                        *redis.Client
	maxTuplesPerWrite             int
	maxTypesPerAuthorizationModel int
}

var _ storage.OpenFGADatastore = (*ValkeyBackend)(nil)

func New(uri string, opts ...ValkeyOption) (*ValkeyBackend, error) {
	opt, err := redis.ParseURL(uri)
	if err != nil {
		return nil, err
	}

	client := redis.NewClient(opt)

	b := &ValkeyBackend{
		client:                        client,
		maxTuplesPerWrite:             storage.DefaultMaxTuplesPerWrite,
		maxTypesPerAuthorizationModel: storage.DefaultMaxTypesPerAuthorizationModel,
	}

	for _, o := range opts {
		o(b)
	}
	return b, nil
}

func WithMaxTuplesPerWrite(max int) ValkeyOption {
	return func(s *ValkeyBackend) {
		s.maxTuplesPerWrite = max
	}
}

func WithMaxTypesPerAuthorizationModel(max int) ValkeyOption {
	return func(s *ValkeyBackend) {
		s.maxTypesPerAuthorizationModel = max
	}
}

// Close closes the datastore and cleans up any residual resources.
func (s *ValkeyBackend) Close() {
	s.client.Close()
}

// IsReady reports whether the datastore is ready to accept traffic.
func (s *ValkeyBackend) IsReady(ctx context.Context) (storage.ReadinessStatus, error) {
	status := storage.ReadinessStatus{
		IsReady: false,
		Message: "Valkey not ready",
	}

	if err := s.client.Ping(ctx).Err(); err != nil {
		status.Message = err.Error()
		return status, err
	}

	status.IsReady = true
	status.Message = "Valkey is ready"
	return status, nil
}

// TupleBackend methods.
func (s *ValkeyBackend) MaxTuplesPerWrite() int {
	return s.maxTuplesPerWrite
}

// AuthorizationModelBackend methods.
func (s *ValkeyBackend) MaxTypesPerAuthorizationModel() int {
	return s.maxTypesPerAuthorizationModel
}
