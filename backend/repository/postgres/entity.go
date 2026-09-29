package postgres

import (
	"time"

	"gorm.io/datatypes"
)

// These rows describe the PostgreSQL tables. They stay inside the repository;
// the service receives only the types from model.
type conversationRow struct {
	ID           string         `gorm:"primaryKey"`
	CustomerJSON datatypes.JSON `gorm:"column:customer_json;type:jsonb"`
	MessagesJSON datatypes.JSON `gorm:"column:messages_json;type:jsonb"`
	CreatedAt    time.Time
}

func (conversationRow) TableName() string { return "conversations" }

type requestRow struct {
	IdempotencyKey string `gorm:"primaryKey"`
	PayloadHash    string
	ConversationID string
	Status         string
	ResponseJSON   datatypes.JSON `gorm:"type:jsonb"`
	ProposalJSON   datatypes.JSON `gorm:"type:jsonb"`
	DecisionJSON   datatypes.JSON `gorm:"type:jsonb"`
	ToolCallsJSON  datatypes.JSON `gorm:"type:jsonb"`
	LeaseUntil     time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (requestRow) TableName() string { return "requests" }

type attemptRow struct {
	OperationKey   string `gorm:"primaryKey"`
	ConversationID string
	RequestKey     string
	Status         string
	ExternalID     string
	ErrorCode      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (attemptRow) TableName() string { return "side_effect_attempts" }
