package repository

import (
	"context"
	"errors"

	"support-triage-service/model"
)

var (
	ErrNotFound   = errors.New("conversation not found")
	ErrConflict   = errors.New("idempotency key reused with different payload")
	ErrInProgress = errors.New("request already in progress")
)

type Reservation struct {
	Key            string
	PayloadHash    string
	ConversationID string
	Customer       *model.Customer
	Messages       []model.Message
}

type Reserved struct {
	ConversationID string
	Completed      bool
	ResponseJSON   []byte
}

// TriageRepository is the persistence contract used by the service.
type TriageRepository interface {
	Reserve(ctx context.Context, request Reservation) (Reserved, error)
	GetConversation(ctx context.Context, id string) (model.Conversation, error)
	SaveToolCall(ctx context.Context, call model.ToolCall) error
	SaveAttempt(ctx context.Context, attempt model.SideEffectAttempt) (model.SideEffectAttempt, error)
	UpdateAttempt(ctx context.Context, attempt model.SideEffectAttempt) error
	Complete(ctx context.Context, requestKey string, proposal model.Proposal, decision model.Decision, responseJSON []byte) error
	Fail(ctx context.Context, requestKey string) error
}

// IncidentIDStore is the persistence contract used by the local incident provider.
type IncidentIDStore interface {
	ClaimIncidentID(ctx context.Context, operationKey, conversationID, proposedID string) (string, error)
	FindIncidentID(ctx context.Context, operationKey string) (string, bool, error)
}
