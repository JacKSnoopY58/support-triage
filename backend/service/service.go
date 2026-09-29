package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"support-triage-service/businesslogic"
	"support-triage-service/model"
	"support-triage-service/repository"
)

var (
	ErrNotFound      = repository.ErrNotFound
	ErrConflict      = repository.ErrConflict
	ErrInProgress    = repository.ErrInProgress
	ErrModel         = errors.New("model unavailable")
	ErrAudit         = errors.New("audit storage unavailable")
	ErrMissingAPIKey = errors.New("OpenAI API key is not configured")
)

type KnowledgeSearch interface {
	Search(ctx context.Context, messages []model.Message) ([]model.KBHit, error)
}

type Model interface {
	Analyze(ctx context.Context, customer model.Customer, messages []model.Message, hits []model.KBHit) (model.Proposal, error)
	Name() string
	PromptVersion() string
}

type IncidentInput struct {
	ConversationID string
	Summary        string
	Urgency        model.Urgency
	Region         string
}

type IncidentResult struct {
	ID     string
	Status string
}

type IncidentGateway interface {
	Open(ctx context.Context, operationKey string, input IncidentInput) (IncidentResult, error)
	Lookup(ctx context.Context, operationKey string) (IncidentResult, bool, error)
}

type Clock interface {
	Now() time.Time
}

type MessageDraft struct {
	Role       string    `json:"role"`
	Body       string    `json:"body"`
	OccurredAt time.Time `json:"occurred_at"`
}

type TicketCommand struct {
	Customer model.Customer `json:"customer"`
	Messages []MessageDraft `json:"messages"`
}

type TurnCommand struct {
	Body       string    `json:"body"`
	OccurredAt time.Time `json:"occurred_at"`
}

type TurnResponse struct {
	ConversationID string         `json:"conversation_id"`
	RequestID      string         `json:"request_id"`
	Reply          string         `json:"reply"`
	Decision       model.Decision `json:"decision"`
}

type Service struct {
	repo      repository.TriageRepository
	kb        KnowledgeSearch
	model     Model
	incidents IncidentGateway
	clock     Clock
	logger    *slog.Logger
}

func NewService(repo repository.TriageRepository, kb KnowledgeSearch, model Model, incidents IncidentGateway, clock Clock, logger *slog.Logger) *Service {
	return &Service{
		repo: repo, kb: kb, model: model, incidents: incidents, clock: clock, logger: logger,
	}
}

func (s *Service) Ingest(ctx context.Context, key string, command TicketCommand) (TurnResponse, error) {
	payloadHash, err := hashPayload(command)
	if err != nil {
		return TurnResponse{}, fmt.Errorf("hashing ticket: %w", err)
	}
	messages := make([]model.Message, 0, len(command.Messages))
	for _, draft := range command.Messages {
		messages = append(messages, model.Message{
			ID: uuid.NewString(), Role: draft.Role, Body: draft.Body, OccurredAt: draft.OccurredAt,
		})
	}
	customer := command.Customer
	return s.process(ctx, repository.Reservation{
		Key: key, PayloadHash: payloadHash, ConversationID: uuid.NewString(),
		Customer: &customer, Messages: messages,
	})
}

func (s *Service) Continue(ctx context.Context, id, key string, command TurnCommand) (TurnResponse, error) {
	payloadHash, err := hashPayload(struct {
		ConversationID string      `json:"conversation_id"`
		Command        TurnCommand `json:"command"`
	}{ConversationID: id, Command: command})
	if err != nil {
		return TurnResponse{}, fmt.Errorf("hashing turn: %w", err)
	}
	return s.process(ctx, repository.Reservation{
		Key: key, PayloadHash: payloadHash, ConversationID: id,
		Messages: []model.Message{{
			ID: uuid.NewString(), Role: "operator", Body: command.Body, OccurredAt: command.OccurredAt,
		}},
	})
}

func (s *Service) Get(ctx context.Context, id string) (model.Conversation, error) {
	return s.repo.GetConversation(ctx, id)
}

func (s *Service) process(ctx context.Context, request repository.Reservation) (response TurnResponse, err error) {
	reserved, err := s.repo.Reserve(ctx, request)
	if err != nil {
		return TurnResponse{}, err
	}
	if reserved.Completed {
		if err := json.Unmarshal(reserved.ResponseJSON, &response); err != nil {
			return TurnResponse{}, fmt.Errorf("decoding replay response: %w", err)
		}
		return response, nil
	}
	defer func() {
		if err != nil {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
			defer cancel()
			if failErr := s.repo.Fail(cleanupCtx, request.Key); failErr != nil {
				s.logger.ErrorContext(ctx, "request_failure_record_failed",
					"request_key", request.Key, "error", failErr)
			}
		}
	}()

	conversation, err := s.repo.GetConversation(ctx, reserved.ConversationID)
	if err != nil {
		return TurnResponse{}, err
	}

	hits, searchErr := s.kb.Search(ctx, conversation.Messages)
	searchCall := model.ToolCall{
		ID: uuid.NewString(), ConversationID: conversation.ID, RequestKey: request.Key,
		Name: "search_knowledge_base", Status: "completed", CreatedAt: s.clock.Now(),
	}
	searchCall.InputJSON = mustJSON(map[string]any{"message_count": len(conversation.Messages)})
	if searchErr != nil {
		searchCall.Status = "failed"
		searchCall.ErrorCode = "knowledge_search_failed"
		hits = []model.KBHit{}
	} else {
		searchCall.OutputJSON = mustJSON(hits)
	}
	if err := s.repo.SaveToolCall(ctx, searchCall); err != nil {
		return TurnResponse{}, fmt.Errorf("%w: saving FAQ search: %v", ErrAudit, err)
	}
	if searchErr != nil {
		s.logger.WarnContext(ctx, "knowledge_search_failed", "conversation_id", conversation.ID, "error", searchErr)
	}

	modelCall := model.ToolCall{
		ID: uuid.NewString(), ConversationID: conversation.ID, RequestKey: request.Key,
		Name: "analyze_ticket", Status: "completed", CreatedAt: s.clock.Now(),
		InputJSON: mustJSON(map[string]any{
			"model": s.model.Name(), "prompt_version": s.model.PromptVersion(),
			"message_ids": messageIDs(conversation.Messages), "faq_hit_ids": hitIDs(hits),
		}),
	}
	proposal, modelErr := s.model.Analyze(ctx, conversation.Customer, conversation.Messages, hits)
	if modelErr == nil {
		modelErr = businesslogic.ValidateProposal(proposal)
	}
	if modelErr != nil {
		modelCall.Status = "failed"
		modelCall.ErrorCode = "model_decision_failed"
	} else {
		modelCall.OutputJSON = mustJSON(proposal)
	}
	if err := s.repo.SaveToolCall(ctx, modelCall); err != nil {
		return TurnResponse{}, fmt.Errorf("%w: saving model call: %v", ErrAudit, err)
	}
	if modelErr != nil {
		if errors.Is(modelErr, ErrMissingAPIKey) {
			return TurnResponse{}, modelErr
		}
		return TurnResponse{}, fmt.Errorf("%w: %v", ErrModel, modelErr)
	}
	proposal.FAQIDs = businesslogic.FilterFAQIDs(proposal.FAQIDs, hits)
	policy := businesslogic.ApplyPolicy(conversation.Customer, proposal, hits)

	decision := model.Decision{
		ID: uuid.NewString(), ConversationID: conversation.ID,
		Urgency: proposal.Urgency, ProductArea: proposal.ProductArea,
		PrimaryIssueType: proposal.PrimaryIssueType, IssueTypes: proposal.IssueTypes,
		CustomerSentiment: proposal.CustomerSentiment, Language: proposal.Language,
		ProposedAction: proposal.Action, Action: policy.Action,
		RequiresHumanApproval: policy.RequiresHumanApproval,
		Rationale:             strings.TrimSpace(proposal.Rationale) + " Policy: " + policy.Reason,
		Reply:                 proposal.Reply, KBHits: hits, FAQIDs: proposal.FAQIDs,
		ToolCallIDs:   []string{searchCall.ID, modelCall.ID},
		PromptVersion: s.model.PromptVersion(), Model: s.model.Name(),
		CreatedAt: s.clock.Now(),
	}
	if policy.OpenIncident {
		callID, err := s.openIncident(ctx, request.Key, conversation, &decision)
		if err != nil {
			return TurnResponse{}, err
		}
		if callID != "" {
			decision.ToolCallIDs = append(decision.ToolCallIDs, callID)
		}
	}

	response = TurnResponse{
		ConversationID: conversation.ID, RequestID: uuid.NewString(),
		Reply: decision.Reply, Decision: decision,
	}
	responseJSON, marshalErr := json.Marshal(response)
	if marshalErr != nil {
		return TurnResponse{}, fmt.Errorf("encoding response: %w", marshalErr)
	}
	if err := s.repo.Complete(ctx, request.Key, proposal, decision, responseJSON); err != nil {
		return TurnResponse{}, fmt.Errorf("completing request: %w", err)
	}
	s.logger.InfoContext(ctx, "triage_decision",
		"conversation_id", conversation.ID, "decision_id", decision.ID,
		"request_key", request.Key, "urgency", decision.Urgency,
		"proposed_action", decision.ProposedAction, "action", decision.Action,
		"incident_status", decision.IncidentStatus)
	return response, nil
}

func (s *Service) openIncident(ctx context.Context, requestKey string, conversation model.Conversation, decision *model.Decision) (string, error) {
	operationKey := "incident:" + conversation.ID
	attempt, err := s.repo.SaveAttempt(ctx, model.SideEffectAttempt{
		OperationKey: operationKey, ConversationID: conversation.ID,
		RequestKey: requestKey, Status: "pending", CreatedAt: s.clock.Now(),
		UpdatedAt: s.clock.Now(),
	})
	if err != nil {
		return "", fmt.Errorf("%w: saving incident attempt: %v", ErrAudit, err)
	}
	if attempt.Status == "completed" {
		decision.IncidentStatus = "completed"
		decision.IncidentID = attempt.ExternalID
		return "", nil
	}

	input := IncidentInput{
		ConversationID: conversation.ID,
		Summary:        decision.Rationale,
		Urgency:        decision.Urgency,
		Region:         conversation.Customer.Region,
	}
	result, callErr := s.incidents.Open(ctx, operationKey, input)
	call := model.ToolCall{
		ID: uuid.NewString(), ConversationID: conversation.ID, RequestKey: requestKey,
		Name: "open_incident", Status: "completed",
		InputJSON: mustJSON(map[string]any{"operation_key": operationKey, "incident": input}),
		CreatedAt: s.clock.Now(),
	}
	if callErr != nil {
		call.Status = "unknown"
		call.ErrorCode = "incident_open_uncertain"
		found, exists, lookupErr := s.incidents.Lookup(ctx, operationKey)
		if lookupErr == nil && exists {
			result = found
			call.Status = "completed"
			call.ErrorCode = ""
		} else if lookupErr != nil {
			s.logger.WarnContext(ctx, "incident_lookup_failed", "operation_key", operationKey, "error", lookupErr)
		}
	}
	if result.ID != "" {
		call.OutputJSON = mustJSON(result)
		attempt.Status = "completed"
		attempt.ExternalID = result.ID
		decision.IncidentStatus = "completed"
		decision.IncidentID = result.ID
	} else {
		attempt.Status = "unknown"
		attempt.ErrorCode = "incident_open_uncertain"
		decision.IncidentStatus = "unknown"
	}
	attempt.UpdatedAt = s.clock.Now()
	if err := s.repo.UpdateAttempt(ctx, attempt); err != nil {
		return "", fmt.Errorf("%w: updating incident attempt: %v", ErrAudit, err)
	}
	if err := s.repo.SaveToolCall(ctx, call); err != nil {
		return "", fmt.Errorf("%w: saving incident tool call: %v", ErrAudit, err)
	}
	return call.ID, nil
}

func hashPayload(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func mustJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func messageIDs(messages []model.Message) []string {
	ids := make([]string, 0, len(messages))
	for _, message := range messages {
		ids = append(ids, message.ID)
	}
	return ids
}

func hitIDs(hits []model.KBHit) []string {
	ids := make([]string, 0, len(hits))
	for _, hit := range hits {
		ids = append(ids, hit.ID)
	}
	return ids
}

type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now().UTC() }
