package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	gormlogger "gorm.io/gorm/logger"
	"support-triage-service/model"
	"support-triage-service/repository"
)

type Store struct {
	db *gorm.DB
}

func Open(dsn string) (*Store, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		TranslateError: true,
		Logger:         gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("opening PostgreSQL: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("accessing connection pool: %w", err)
	}
	sqlDB.SetMaxOpenConns(15)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)
	sqlDB.SetConnMaxIdleTime(time.Minute)
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("pinging PostgreSQL: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Reserve(ctx context.Context, input repository.Reservation) (repository.Reserved, error) {
	for range 2 {
		result, err := s.reserveOnce(ctx, input)
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			continue
		}
		return result, err
	}
	return repository.Reserved{}, repository.ErrInProgress
}

func (s *Store) reserveOnce(ctx context.Context, input repository.Reservation) (repository.Reserved, error) {
	result := repository.Reserved{}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing requestRow
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("idempotency_key = ?", input.Key).Take(&existing).Error
		if err == nil {
			if existing.PayloadHash != input.PayloadHash {
				return repository.ErrConflict
			}
			result.ConversationID = existing.ConversationID
			if existing.Status == "completed" {
				result.Completed = true
				result.ResponseJSON = append([]byte(nil), existing.ResponseJSON...)
				return nil
			}
			if existing.Status == "processing" && time.Now().Before(existing.LeaseUntil) {
				return repository.ErrInProgress
			}
			if err := tx.Model(&existing).Updates(map[string]any{
				"status": "processing", "lease_until": time.Now().Add(4 * time.Minute),
				"updated_at": time.Now(),
			}).Error; err != nil {
				return fmt.Errorf("claiming retry: %w", err)
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("checking request: %w", err)
		}

		messagesJSON, err := json.Marshal(input.Messages)
		if err != nil {
			return fmt.Errorf("encoding messages: %w", err)
		}
		if input.Customer != nil {
			customerJSON, err := json.Marshal(input.Customer)
			if err != nil {
				return fmt.Errorf("encoding customer: %w", err)
			}
			if err := tx.Create(&conversationRow{
				ID: input.ConversationID, CustomerJSON: customerJSON,
				MessagesJSON: messagesJSON, CreatedAt: time.Now().UTC(),
			}).Error; err != nil {
				return fmt.Errorf("creating conversation: %w", err)
			}
		} else {
			update := tx.Model(&conversationRow{}).Where("id = ?", input.ConversationID).
				Update("messages_json", gorm.Expr("messages_json || CAST(? AS jsonb)", string(messagesJSON)))
			if update.Error != nil {
				return fmt.Errorf("appending messages: %w", update.Error)
			}
			if update.RowsAffected != 1 {
				return repository.ErrNotFound
			}
		}
		req := requestRow{
			IdempotencyKey: input.Key, PayloadHash: input.PayloadHash,
			ConversationID: input.ConversationID, Status: "processing",
			ToolCallsJSON: datatypes.JSON("[]"), LeaseUntil: time.Now().Add(4 * time.Minute),
		}
		if err := tx.Create(&req).Error; err != nil {
			return fmt.Errorf("reserving request: %w", err)
		}
		result.ConversationID = input.ConversationID
		return nil
	})
	if err != nil {
		return repository.Reserved{}, err
	}
	return result, nil
}

func (s *Store) GetConversation(ctx context.Context, id string) (model.Conversation, error) {
	var row conversationRow
	err := s.db.WithContext(ctx).Where("id = ?", id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Conversation{}, repository.ErrNotFound
	}
	if err != nil {
		return model.Conversation{}, fmt.Errorf("loading conversation: %w", err)
	}
	result, err := row.toModel()
	if err != nil {
		return model.Conversation{}, err
	}
	var requests []requestRow
	if err := s.db.WithContext(ctx).Where("conversation_id = ?", id).
		Order("created_at, idempotency_key").Find(&requests).Error; err != nil {
		return model.Conversation{}, fmt.Errorf("loading request history: %w", err)
	}
	if err := appendRequestRows(&result, requests); err != nil {
		return model.Conversation{}, err
	}
	var attempts []attemptRow
	if err := s.db.WithContext(ctx).Where("conversation_id = ?", id).
		Order("created_at, operation_key").Find(&attempts).Error; err != nil {
		return model.Conversation{}, fmt.Errorf("loading side effects: %w", err)
	}
	appendAttemptRows(&result, attempts)
	return result, nil
}

func (s *Store) SaveToolCall(ctx context.Context, call model.ToolCall) error {
	data, err := json.Marshal([]model.ToolCall{call})
	if err != nil {
		return fmt.Errorf("encoding tool call: %w", err)
	}
	update := s.db.WithContext(ctx).Model(&requestRow{}).
		Where("idempotency_key = ? AND conversation_id = ?", call.RequestKey, call.ConversationID).
		Update("tool_calls_json", gorm.Expr("tool_calls_json || CAST(? AS jsonb)", string(data)))
	if update.Error != nil {
		return fmt.Errorf("saving tool call: %w", update.Error)
	}
	if update.RowsAffected != 1 {
		return fmt.Errorf("request disappeared while saving tool call")
	}
	return nil
}

func (s *Store) SaveAttempt(ctx context.Context, attempt model.SideEffectAttempt) (model.SideEffectAttempt, error) {
	row := attemptRowFromModel(attempt)
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).
		Create(&row).Error; err != nil {
		return model.SideEffectAttempt{}, err
	}
	if err := s.db.WithContext(ctx).Where("operation_key = ?", attempt.OperationKey).
		Take(&row).Error; err != nil {
		return model.SideEffectAttempt{}, err
	}
	return row.toModel(), nil
}

func (s *Store) UpdateAttempt(ctx context.Context, attempt model.SideEffectAttempt) error {
	updates := map[string]any{
		"status":     gorm.Expr("CASE WHEN status = 'completed' THEN status ELSE ? END", attempt.Status),
		"error_code": gorm.Expr("CASE WHEN status = 'completed' THEN error_code ELSE ? END", attempt.ErrorCode),
		"updated_at": attempt.UpdatedAt,
	}
	if attempt.ExternalID != "" {
		updates["external_id"] = attempt.ExternalID
	}
	return s.db.WithContext(ctx).Model(&attemptRow{}).
		Where("operation_key = ?", attempt.OperationKey).Updates(updates).Error
}

func (s *Store) Complete(ctx context.Context, requestKey string, proposal model.Proposal, decision model.Decision, responseJSON []byte) error {
	proposalJSON, err := json.Marshal(proposal)
	if err != nil {
		return fmt.Errorf("encoding proposal: %w", err)
	}
	decisionJSON, err := json.Marshal(decision)
	if err != nil {
		return fmt.Errorf("encoding decision: %w", err)
	}
	assistantJSON, err := json.Marshal([]model.Message{{
		ID: decision.ID + ":reply", Role: "assistant",
		Body: decision.Reply, OccurredAt: decision.CreatedAt,
	}})
	if err != nil {
		return fmt.Errorf("encoding assistant reply: %w", err)
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		update := tx.Model(&requestRow{}).
			Where("idempotency_key = ? AND conversation_id = ? AND status = ? AND decision_json IS NULL",
				requestKey, decision.ConversationID, "processing").
			Updates(map[string]any{
				"status": "completed", "response_json": datatypes.JSON(responseJSON),
				"proposal_json": datatypes.JSON(proposalJSON),
				"decision_json": datatypes.JSON(decisionJSON),
				"updated_at":    time.Now().UTC(),
			})
		if update.Error != nil {
			return fmt.Errorf("saving decision and replay response: %w", update.Error)
		}
		if update.RowsAffected != 1 {
			return fmt.Errorf("request was already completed or disappeared")
		}
		update = tx.Model(&conversationRow{}).Where("id = ?", decision.ConversationID).
			Update("messages_json", gorm.Expr("messages_json || CAST(? AS jsonb)", string(assistantJSON)))
		if update.Error != nil {
			return fmt.Errorf("saving assistant reply: %w", update.Error)
		}
		if update.RowsAffected != 1 {
			return fmt.Errorf("conversation disappeared during completion")
		}
		return nil
	})
}

func (s *Store) Fail(ctx context.Context, requestKey string) error {
	return s.db.WithContext(ctx).Model(&requestRow{}).
		Where("idempotency_key = ? AND status = ?", requestKey, "processing").
		Updates(map[string]any{"status": "failed", "updated_at": time.Now().UTC()}).Error
}

func (s *Store) ClaimIncidentID(ctx context.Context, operationKey, conversationID, proposedID string) (string, error) {
	update := s.db.WithContext(ctx).Table("side_effect_attempts").
		Where("operation_key = ? AND conversation_id = ?", operationKey, conversationID).
		Update("external_id", gorm.Expr("COALESCE(NULLIF(external_id, ''), ?)", proposedID))
	if update.Error != nil {
		return "", fmt.Errorf("claiming incident ID: %w", update.Error)
	}
	if update.RowsAffected != 1 {
		return "", errors.New("incident attempt was not reserved")
	}
	id, found, err := s.FindIncidentID(ctx, operationKey)
	if err != nil {
		return "", err
	}
	if !found {
		return "", errors.New("incident has no external ID")
	}
	return id, nil
}

func (s *Store) FindIncidentID(ctx context.Context, operationKey string) (string, bool, error) {
	var row struct{ ExternalID *string }
	err := s.db.WithContext(ctx).Table("side_effect_attempts").Select("external_id").
		Where("operation_key = ?", operationKey).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("looking up incident ID: %w", err)
	}
	if row.ExternalID == nil || *row.ExternalID == "" {
		return "", false, nil
	}
	return *row.ExternalID, true, nil
}

var _ repository.TriageRepository = (*Store)(nil)
var _ repository.IncidentIDStore = (*Store)(nil)
