package model

import "time"

type Urgency string
type Action string

const (
	UrgencyCritical Urgency = "critical"
	UrgencyHigh     Urgency = "high"
	UrgencyMedium   Urgency = "medium"
	UrgencyLow      Urgency = "low"

	ActionAutoRespond       Action = "auto_respond"
	ActionRouteToSpecialist Action = "route_to_specialist"
	ActionEscalateToHuman   Action = "escalate_to_human"
)

type Customer struct {
	Plan         string `json:"plan"`
	Region       string `json:"region"`
	Seats        int    `json:"seats"`
	TenureMonths int    `json:"tenure_months"`
	PriorTickets int    `json:"prior_tickets"`
}

type Message struct {
	ID         string    `json:"id"`
	Role       string    `json:"role"`
	Body       string    `json:"body"`
	OccurredAt time.Time `json:"occurred_at"`
}

type KBHit struct {
	ID      string  `json:"id"`
	Title   string  `json:"title"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

type Proposal struct {
	Urgency           Urgency  `json:"urgency"`
	ProductArea       string   `json:"product_area"`
	PrimaryIssueType  string   `json:"primary_issue_type"`
	IssueTypes        []string `json:"issue_types"`
	CustomerSentiment string   `json:"customer_sentiment"`
	Language          string   `json:"language"`
	Action            Action   `json:"action"`
	Rationale         string   `json:"rationale"`
	Reply             string   `json:"reply"`
	FAQIDs            []string `json:"faq_ids"`
}

type Decision struct {
	ID                    string    `json:"id"`
	ConversationID        string    `json:"conversation_id"`
	Urgency               Urgency   `json:"urgency"`
	ProductArea           string    `json:"product_area"`
	PrimaryIssueType      string    `json:"primary_issue_type"`
	IssueTypes            []string  `json:"issue_types"`
	CustomerSentiment     string    `json:"customer_sentiment"`
	Language              string    `json:"language"`
	ProposedAction        Action    `json:"proposed_action"`
	Action                Action    `json:"action"`
	RequiresHumanApproval bool      `json:"requires_human_approval"`
	Rationale             string    `json:"rationale"`
	Reply                 string    `json:"reply"`
	KBHits                []KBHit   `json:"kb_hits"`
	FAQIDs                []string  `json:"faq_ids"`
	ToolCallIDs           []string  `json:"tool_call_ids"`
	IncidentStatus        string    `json:"incident_status,omitempty"`
	IncidentID            string    `json:"incident_id,omitempty"`
	PromptVersion         string    `json:"prompt_version"`
	Model                 string    `json:"model"`
	CreatedAt             time.Time `json:"created_at"`
}

type ToolCall struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	RequestKey     string    `json:"request_key"`
	Name           string    `json:"name"`
	Status         string    `json:"status"`
	InputJSON      string    `json:"input_json"`
	OutputJSON     string    `json:"output_json,omitempty"`
	ErrorCode      string    `json:"error_code,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type SideEffectAttempt struct {
	OperationKey   string    `json:"operation_key"`
	ConversationID string    `json:"conversation_id"`
	RequestKey     string    `json:"request_key"`
	Status         string    `json:"status"`
	ExternalID     string    `json:"external_id,omitempty"`
	ErrorCode      string    `json:"error_code,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Conversation struct {
	ID          string              `json:"id"`
	Customer    Customer            `json:"customer"`
	Messages    []Message           `json:"messages"`
	Decisions   []Decision          `json:"decisions"`
	ToolCalls   []ToolCall          `json:"tool_calls"`
	SideEffects []SideEffectAttempt `json:"side_effect_attempts"`
	CreatedAt   time.Time           `json:"created_at"`
}
