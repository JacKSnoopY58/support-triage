//go:build integration

package incident_test

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"support-triage-service/model"
	"support-triage-service/providers/incident"
	"support-triage-service/repository"
	"support-triage-service/repository/postgres"
	"support-triage-service/service"
)

func TestConcurrentOpenUsesOneDurableIncident(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	store, err := postgres.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	conversationID := uuid.NewString()
	requestKey := "incident-race-" + uuid.NewString()
	customer := model.Customer{Plan: "Enterprise", Seats: 2}
	if _, err := store.Reserve(ctx, repository.Reservation{
		Key: requestKey, PayloadHash: "race", ConversationID: conversationID,
		Customer: &customer, Messages: []model.Message{},
	}); err != nil {
		t.Fatal(err)
	}
	operationKey := "incident:" + conversationID
	if _, err := store.SaveAttempt(ctx, model.SideEffectAttempt{
		OperationKey: operationKey, ConversationID: conversationID,
		RequestKey: requestKey, Status: "pending", CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	const callers = 8
	type outcome struct {
		id  string
		err error
	}
	results := make(chan outcome, callers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			result, err := incident.New(store, 0, "").Open(ctx, operationKey,
				service.IncidentInput{ConversationID: conversationID})
			results <- outcome{id: result.ID, err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	var firstID string
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent open: %v", result.err)
		}
		if result.id == "" {
			t.Fatal("concurrent open returned no incident ID")
		}
		if firstID == "" {
			firstID = result.id
		} else if result.id != firstID {
			t.Fatalf("duplicate incident IDs: %s and %s", firstID, result.id)
		}
	}
	reopened, err := postgres.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	stored, found, err := incident.New(reopened, 0, "").Lookup(ctx, operationKey)
	if err != nil || !found || stored.ID != firstID {
		t.Fatalf("lookup after reconnect: result=%+v found=%t err=%v", stored, found, err)
	}
}
