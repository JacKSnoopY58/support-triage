package businesslogic

import (
	"testing"

	"support-triage-service/model"
)

func TestApplyPolicy(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		customer     model.Customer
		proposal     model.Proposal
		hits         []model.KBHit
		wantAction   model.Action
		wantIncident bool
	}{
		{
			name:       "billing cannot refund or auto respond",
			customer:   model.Customer{Plan: "Free", Seats: 1},
			proposal:   validProposal("billing", model.UrgencyHigh, model.ActionAutoRespond),
			hits:       []model.KBHit{{ID: "billing", Score: 1}},
			wantAction: model.ActionEscalateToHuman,
		},
		{
			name:         "enterprise outage opens incident",
			customer:     model.Customer{Plan: "Enterprise", Seats: 45},
			proposal:     validProposal("outage", model.UrgencyCritical, model.ActionEscalateToHuman),
			wantAction:   model.ActionEscalateToHuman,
			wantIncident: true,
		},
		{
			name:       "single user critical issue does not page provider",
			customer:   model.Customer{Plan: "Pro", Seats: 1},
			proposal:   validProposal("outage", model.UrgencyCritical, model.ActionEscalateToHuman),
			wantAction: model.ActionEscalateToHuman,
		},
		{
			name:       "routine question with FAQ can auto respond",
			customer:   model.Customer{Plan: "Pro", Seats: 1},
			proposal:   validProposal("feature_question", model.UrgencyLow, model.ActionAutoRespond),
			hits:       []model.KBHit{{ID: "appearance", Score: 0.8}},
			wantAction: model.ActionAutoRespond,
		},
		{
			name:       "bug is routed despite model auto response",
			customer:   model.Customer{Plan: "Pro", Seats: 1},
			proposal:   validProposal("bug", model.UrgencyLow, model.ActionAutoRespond),
			hits:       []model.KBHit{{ID: "appearance", Score: 0.8}},
			wantAction: model.ActionRouteToSpecialist,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := ApplyPolicy(test.customer, test.proposal, test.hits)
			if got.Action != test.wantAction || got.OpenIncident != test.wantIncident {
				t.Fatalf("got action=%s incident=%t, want action=%s incident=%t",
					got.Action, got.OpenIncident, test.wantAction, test.wantIncident)
			}
		})
	}
}

func TestValidateProposalRejectsUnknownEnum(t *testing.T) {
	t.Parallel()
	proposal := validProposal("other", model.UrgencyLow, model.ActionAutoRespond)
	proposal.Urgency = "immediate"
	if err := ValidateProposal(proposal); err == nil {
		t.Fatal("expected invalid urgency to fail validation")
	}
}

func validProposal(issue string, urgency model.Urgency, action model.Action) model.Proposal {
	return model.Proposal{
		Urgency: urgency, ProductArea: "app", PrimaryIssueType: issue,
		IssueTypes: []string{issue}, CustomerSentiment: "neutral", Language: "en",
		Action: action, Rationale: "ticket evidence", Reply: "We are reviewing this.",
		FAQIDs: []string{},
	}
}
