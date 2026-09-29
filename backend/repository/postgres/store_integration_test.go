//go:build integration

package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"support-triage-service/model"
	"support-triage-service/repository"
	"support-triage-service/repository/postgres"
)

func TestStoreReturnsServiceModelsFromDatabaseRows(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	store, err := postgres.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	conversationID := uuid.NewString()
	requestKey := "mapping-" + uuid.NewString()
	customer := model.Customer{Plan: "Pro", Region: "Thailand", Seats: 3}
	message := model.Message{
		ID: uuid.NewString(), Role: "customer", Body: "Cannot sign in", OccurredAt: now,
	}
	if _, err := store.Reserve(ctx, repository.Reservation{
		Key: requestKey, PayloadHash: "mapping-test", ConversationID: conversationID,
		Customer: &customer, Messages: []model.Message{message},
	}); err != nil {
		t.Fatal(err)
	}
	toolCall := model.ToolCall{
		ID: uuid.NewString(), ConversationID: conversationID, RequestKey: requestKey,
		Name: "search_knowledge_base", Status: "completed", InputJSON: "{}",
		OutputJSON: "[]", CreatedAt: now,
	}
	if err := store.SaveToolCall(ctx, toolCall); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveAttempt(ctx, model.SideEffectAttempt{
		OperationKey: "incident:" + conversationID, ConversationID: conversationID,
		RequestKey: requestKey, Status: "pending", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	proposal := model.Proposal{Urgency: model.UrgencyHigh, Action: model.ActionEscalateToHuman}
	decision := model.Decision{
		ID: uuid.NewString(), ConversationID: conversationID,
		Urgency: proposal.Urgency, Action: proposal.Action,
		Reply: "We will review this case.", CreatedAt: now.Add(time.Second),
	}
	if err := store.Complete(ctx, requestKey, proposal, decision, []byte(`{"ok":true}`)); err != nil {
		t.Fatal(err)
	}

	got, err := store.GetConversation(ctx, conversationID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Customer != customer || len(got.Messages) != 2 ||
		got.Messages[0].ID != message.ID || got.Messages[1].Body != decision.Reply {
		t.Fatalf("conversation mapping lost customer or messages: %+v", got)
	}
	if len(got.Decisions) != 1 || got.Decisions[0].ID != decision.ID ||
		len(got.ToolCalls) != 1 || got.ToolCalls[0].ID != toolCall.ID ||
		len(got.SideEffects) != 1 || got.SideEffects[0].OperationKey != "incident:"+conversationID {
		t.Fatalf("conversation mapping lost audit data: %+v", got)
	}
}
