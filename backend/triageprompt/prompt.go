package triageprompt

import _ "embed"

const Version = "triage"

//go:embed triage.md
var Instructions string

// ProposalSchema defines the structured output shared by model adapters.
func ProposalSchema() map[string]any {
	issueValues := []string{"billing", "outage", "bug", "feature_question", "feature_request", "mixed", "other"}
	properties := map[string]any{
		"urgency":            map[string]any{"type": "string", "enum": []string{"critical", "high", "medium", "low"}},
		"product_area":       map[string]any{"type": "string"},
		"primary_issue_type": map[string]any{"type": "string", "enum": issueValues},
		"issue_types":        map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": issueValues}},
		"customer_sentiment": map[string]any{"type": "string", "enum": []string{"positive", "neutral", "frustrated", "angry"}},
		"language":           map[string]any{"type": "string"},
		"action":             map[string]any{"type": "string", "enum": []string{"auto_respond", "route_to_specialist", "escalate_to_human"}},
		"rationale":          map[string]any{"type": "string"},
		"reply":              map[string]any{"type": "string"},
		"faq_ids":            map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
	}
	return map[string]any{
		"type":       "object",
		"properties": properties,
		"required": []string{
			"urgency", "product_area", "primary_issue_type", "issue_types",
			"customer_sentiment", "language", "action", "rationale", "reply", "faq_ids",
		},
		"additionalProperties": false,
	}
}
