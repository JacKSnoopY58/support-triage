package businesslogic

import (
	"errors"
	"fmt"
	"strings"

	"support-triage-service/model"
)

var ErrInvalidProposal = errors.New("invalid model proposal")

var issueTypes = map[string]bool{
	"billing":          true,
	"outage":           true,
	"bug":              true,
	"feature_question": true,
	"feature_request":  true,
	"other":            true,
	"mixed":            true,
}

func ValidateProposal(p model.Proposal) error {
	if p.Urgency != model.UrgencyCritical && p.Urgency != model.UrgencyHigh &&
		p.Urgency != model.UrgencyMedium && p.Urgency != model.UrgencyLow {
		return fmt.Errorf("%w: urgency", ErrInvalidProposal)
	}
	if p.Action != model.ActionAutoRespond && p.Action != model.ActionRouteToSpecialist &&
		p.Action != model.ActionEscalateToHuman {
		return fmt.Errorf("%w: action", ErrInvalidProposal)
	}
	if !issueTypes[p.PrimaryIssueType] {
		return fmt.Errorf("%w: primary_issue_type", ErrInvalidProposal)
	}
	if len(p.IssueTypes) == 0 {
		return fmt.Errorf("%w: issue_types", ErrInvalidProposal)
	}
	for _, issue := range p.IssueTypes {
		if !issueTypes[issue] {
			return fmt.Errorf("%w: issue_types", ErrInvalidProposal)
		}
	}
	if strings.TrimSpace(p.ProductArea) == "" || strings.TrimSpace(p.Language) == "" ||
		strings.TrimSpace(p.Rationale) == "" || strings.TrimSpace(p.Reply) == "" {
		return fmt.Errorf("%w: required text", ErrInvalidProposal)
	}
	switch p.CustomerSentiment {
	case "positive", "neutral", "frustrated", "angry":
	default:
		return fmt.Errorf("%w: customer_sentiment", ErrInvalidProposal)
	}
	if len(p.Reply) > 4000 || len(p.Rationale) > 4000 {
		return fmt.Errorf("%w: text too long", ErrInvalidProposal)
	}
	return nil
}

func HasIssue(p model.Proposal, issue string) bool {
	if p.PrimaryIssueType == issue {
		return true
	}
	for _, item := range p.IssueTypes {
		if item == issue {
			return true
		}
	}
	return false
}

type PolicyResult struct {
	Action                model.Action
	RequiresHumanApproval bool
	OpenIncident          bool
	Reason                string
}

// ApplyPolicy is the autonomy boundary. The model's suggestion is never a
// capability grant; only these deterministic rules can authorize a side effect.
func ApplyPolicy(customer model.Customer, proposal model.Proposal, hits []model.KBHit) PolicyResult {
	if HasIssue(proposal, "billing") {
		return PolicyResult{
			Action:                model.ActionEscalateToHuman,
			RequiresHumanApproval: true,
			Reason:                "billing state must be checked by a human before financial action",
		}
	}

	if proposal.Urgency == model.UrgencyCritical {
		openIncident := HasIssue(proposal, "outage") &&
			strings.EqualFold(customer.Plan, "enterprise") && customer.Seats >= 2
		return PolicyResult{
			Action:                model.ActionEscalateToHuman,
			RequiresHumanApproval: true,
			OpenIncident:          openIncident,
			Reason:                "critical issue requires human escalation",
		}
	}

	if proposal.Action == model.ActionEscalateToHuman {
		return PolicyResult{
			Action:                model.ActionEscalateToHuman,
			RequiresHumanApproval: true,
			Reason:                "model requested human escalation",
		}
	}

	if proposal.Action == model.ActionAutoRespond &&
		proposal.Urgency == model.UrgencyLow &&
		!HasIssue(proposal, "bug") &&
		len(hits) > 0 && hits[0].Score >= 0.3 {
		return PolicyResult{
			Action: model.ActionAutoRespond,
			Reason: "low-urgency question has a relevant knowledge-base match",
		}
	}

	return PolicyResult{
		Action:                model.ActionRouteToSpecialist,
		RequiresHumanApproval: true,
		Reason:                "specialist should verify a non-routine answer",
	}
}
