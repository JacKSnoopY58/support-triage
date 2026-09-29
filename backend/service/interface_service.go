package service

import (
	"context"

	"support-triage-service/model"
)

type TriageUseCase interface {
	Ingest(ctx context.Context, key string, command TicketCommand) (TurnResponse, error)
	Continue(ctx context.Context, id, key string, command TurnCommand) (TurnResponse, error)
	Get(ctx context.Context, id string) (model.Conversation, error)
}

var _ TriageUseCase = (*Service)(nil)
