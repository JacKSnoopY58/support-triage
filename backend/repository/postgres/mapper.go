package postgres

import (
	"encoding/json"
	"fmt"
	"sort"

	"support-triage-service/model"
)

func (row conversationRow) toModel() (model.Conversation, error) {
	result := model.Conversation{
		ID: row.ID, CreatedAt: row.CreatedAt,
		Messages: []model.Message{}, Decisions: []model.Decision{},
		ToolCalls: []model.ToolCall{}, SideEffects: []model.SideEffectAttempt{},
	}
	if err := json.Unmarshal(row.CustomerJSON, &result.Customer); err != nil {
		return model.Conversation{}, fmt.Errorf("decoding customer: %w", err)
	}
	if err := json.Unmarshal(row.MessagesJSON, &result.Messages); err != nil {
		return model.Conversation{}, fmt.Errorf("decoding messages: %w", err)
	}
	sort.SliceStable(result.Messages, func(i, j int) bool {
		return result.Messages[i].OccurredAt.Before(result.Messages[j].OccurredAt)
	})
	return result, nil
}

func appendRequestRows(result *model.Conversation, rows []requestRow) error {
	for _, row := range rows {
		if len(row.DecisionJSON) > 0 && string(row.DecisionJSON) != "null" {
			var decision model.Decision
			if err := json.Unmarshal(row.DecisionJSON, &decision); err != nil {
				return fmt.Errorf("decoding decision: %w", err)
			}
			result.Decisions = append(result.Decisions, decision)
		}
		var calls []model.ToolCall
		if err := json.Unmarshal(row.ToolCallsJSON, &calls); err != nil {
			return fmt.Errorf("decoding tool calls: %w", err)
		}
		result.ToolCalls = append(result.ToolCalls, calls...)
	}
	sort.Slice(result.Decisions, func(i, j int) bool {
		if result.Decisions[i].CreatedAt.Equal(result.Decisions[j].CreatedAt) {
			return result.Decisions[i].ID < result.Decisions[j].ID
		}
		return result.Decisions[i].CreatedAt.Before(result.Decisions[j].CreatedAt)
	})
	sort.Slice(result.ToolCalls, func(i, j int) bool {
		if result.ToolCalls[i].CreatedAt.Equal(result.ToolCalls[j].CreatedAt) {
			return result.ToolCalls[i].ID < result.ToolCalls[j].ID
		}
		return result.ToolCalls[i].CreatedAt.Before(result.ToolCalls[j].CreatedAt)
	})
	return nil
}

func appendAttemptRows(result *model.Conversation, rows []attemptRow) {
	for _, row := range rows {
		result.SideEffects = append(result.SideEffects, row.toModel())
	}
}

func attemptRowFromModel(attempt model.SideEffectAttempt) attemptRow {
	return attemptRow{
		OperationKey: attempt.OperationKey, ConversationID: attempt.ConversationID,
		RequestKey: attempt.RequestKey, Status: attempt.Status,
		ExternalID: attempt.ExternalID, ErrorCode: attempt.ErrorCode,
		CreatedAt: attempt.CreatedAt, UpdatedAt: attempt.UpdatedAt,
	}
}

func (row attemptRow) toModel() model.SideEffectAttempt {
	return model.SideEffectAttempt{
		OperationKey: row.OperationKey, ConversationID: row.ConversationID,
		RequestKey: row.RequestKey, Status: row.Status, ExternalID: row.ExternalID,
		ErrorCode: row.ErrorCode, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}
