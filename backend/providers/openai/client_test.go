package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"support-triage-service/model"
)

func TestAnalyzeSendsStructuredSchemaAndParsesResponse(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/responses" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}
		var request struct {
			Model string `json:"model"`
			Store bool   `json:"store"`
			Text  struct {
				Format struct {
					Type   string `json:"type"`
					Strict bool   `json:"strict"`
				} `json:"format"`
			} `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if request.Model != "gpt-4o-mini" || request.Store ||
			request.Text.Format.Type != "json_schema" || !request.Text.Format.Strict {
			t.Errorf("unexpected model request: %+v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status":"completed",
			"output":[{"type":"message","content":[{"type":"output_text",
				"text":"{\"urgency\":\"high\",\"product_area\":\"billing\",\"primary_issue_type\":\"billing\",\"issue_types\":[\"billing\"],\"customer_sentiment\":\"frustrated\",\"language\":\"en\",\"action\":\"escalate_to_human\",\"rationale\":\"multiple charges\",\"reply\":\"We are checking this.\",\"faq_ids\":[]}"}]}]
		}`))
	}))
	defer server.Close()

	client := New("test-key", "gpt-4o-mini", server.URL, server.Client())
	got, err := client.Analyze(context.Background(), model.Customer{Plan: "Free", Seats: 1},
		[]model.Message{{Role: "customer", Body: "I see multiple charges"}},
		[]model.KBHit{})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if got.Urgency != model.UrgencyHigh || got.PrimaryIssueType != "billing" {
		t.Fatalf("unexpected proposal: %+v", got)
	}
}

func TestAnalyzeReportsProviderErrorCodeWithoutMessage(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"code":"insufficient_quota","message":"private provider details"}}`))
	}))
	defer server.Close()
	client := New("test-key", "gpt-4o-mini", server.URL, server.Client())
	_, err := client.Analyze(context.Background(), model.Customer{}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "code=insufficient_quota") ||
		strings.Contains(err.Error(), "private provider details") {
		t.Fatalf("unexpected provider error: %v", err)
	}
}
