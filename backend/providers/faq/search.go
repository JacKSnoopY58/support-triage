package faq

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"support-triage-service/model"
	"support-triage-service/service"
)

type document struct {
	id       string
	title    string
	content  string
	keywords []string
}

var documents = []document{
	{
		id:       "billing-pending-charges",
		title:    "Pending charges after an upgrade attempt",
		content:  "A pending authorization is not proof of settlement. Support must compare payment-provider events with account entitlements before issuing a refund or changing access.",
		keywords: []string{"payment", "charge", "billing", "upgrade", "refund", "pending", "card"},
	},
	{
		id:       "outage-regional-access",
		title:    "Investigating regional access failures",
		content:  "Collect region, HTTP errors, affected users and start time. A green status page can lag a regional incident. Escalate widespread access failures for investigation.",
		keywords: []string{"outage", "error 500", "region", "access", "enterprise", "เข้าไม่ได้", "ใช้งานไม่ได้", "หลายเครื่อง"},
	},
	{
		id:       "appearance-settings",
		title:    "Appearance settings and dark mode",
		content:  "Appearance settings may include Light and System Default. If System Default does not follow the operating system setting, collect app version and platform for a bug investigation. Scheduled switching is not documented.",
		keywords: []string{"dark mode", "appearance", "light", "system default", "theme", "schedule"},
	},
	{
		id:       "export-entitlements",
		title:    "Checking Pro export entitlement",
		content:  "Export features require an active Pro entitlement. Check the payment and entitlement records separately; do not promise immediate activation before verification.",
		keywords: []string{"pro", "export", "entitlement", "presentation", "upgrade"},
	},
}

type Searcher struct {
	delay time.Duration
	fail  bool
}

func New(delay time.Duration, fail bool) *Searcher {
	return &Searcher{delay: delay, fail: fail}
}

func (s *Searcher) Search(ctx context.Context, messages []model.Message) ([]model.KBHit, error) {
	if s.delay > 0 {
		timer := time.NewTimer(s.delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	if s.fail {
		return nil, errors.New("knowledge base temporarily unavailable")
	}
	var text strings.Builder
	for _, message := range messages {
		if message.Role == "customer" || message.Role == "operator" {
			text.WriteString(" ")
			text.WriteString(strings.ToLower(message.Body))
		}
	}
	query := text.String()
	hits := []model.KBHit{}
	for _, doc := range documents {
		matches := 0
		for _, word := range doc.keywords {
			if strings.Contains(query, word) {
				matches++
			}
		}
		if matches == 0 {
			continue
		}
		score := float64(matches) / float64(len(doc.keywords))
		hits = append(hits, model.KBHit{
			ID: doc.id, Title: doc.title, Content: doc.content, Score: score,
		})
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	if len(hits) > 3 {
		hits = hits[:3]
	}
	return hits, nil
}

var _ service.KnowledgeSearch = (*Searcher)(nil)
