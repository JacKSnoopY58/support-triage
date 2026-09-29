package incident

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"support-triage-service/repository"
	"support-triage-service/service"
)

// Mock models a provider with an Idempotency-Key contract and lookup endpoint.
// The durable external ID lives in the already-reserved side-effect attempt.
// A real provider would keep its own state outside this database.
type Mock struct {
	store    repository.IncidentIDStore
	delay    time.Duration
	failMode string
}

func New(store repository.IncidentIDStore, delay time.Duration, failMode string) *Mock {
	return &Mock{store: store, delay: delay, failMode: failMode}
}

func (m *Mock) Open(ctx context.Context, operationKey string, input service.IncidentInput) (service.IncidentResult, error) {
	if m.delay > 0 {
		timer := time.NewTimer(m.delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return service.IncidentResult{}, ctx.Err()
		case <-timer.C:
		}
	}
	if m.failMode == "before" {
		return service.IncidentResult{}, errors.New("mock incident provider unavailable before commit")
	}
	id, err := m.store.ClaimIncidentID(ctx, operationKey, input.ConversationID, "inc_"+uuid.NewString())
	if err != nil {
		return service.IncidentResult{}, fmt.Errorf("opening mock incident: %w", err)
	}
	if m.failMode == "after" {
		return service.IncidentResult{}, errors.New("mock response lost after incident commit")
	}
	if m.failMode == "misleading" {
		return service.IncidentResult{ID: id, Status: "all_systems_operational"}, nil
	}
	return service.IncidentResult{ID: id, Status: "opened"}, nil
}

func (m *Mock) Lookup(ctx context.Context, operationKey string) (service.IncidentResult, bool, error) {
	id, found, err := m.store.FindIncidentID(ctx, operationKey)
	if err != nil {
		return service.IncidentResult{}, false, fmt.Errorf("looking up mock incident: %w", err)
	}
	if !found {
		return service.IncidentResult{}, false, nil
	}
	return service.IncidentResult{ID: id, Status: "opened"}, true, nil
}

var _ service.IncidentGateway = (*Mock)(nil)
