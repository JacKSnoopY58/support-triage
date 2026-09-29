package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"support-triage-service/businesslogic"
	"support-triage-service/model"
	"support-triage-service/service"
	"support-triage-service/triageprompt"
)

type Client struct {
	key     string
	model   string
	baseURL string
	http    *http.Client
}

func New(key, model, baseURL string, client *http.Client) *Client {
	if model == "" {
		model = "gpt-4o-mini"
	}
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if client == nil {
		client = &http.Client{Timeout: 35 * time.Second}
	}
	return &Client{key: key, model: model, baseURL: strings.TrimRight(baseURL, "/"), http: client}
}

func (c *Client) Name() string          { return c.model }
func (c *Client) PromptVersion() string { return triageprompt.Version }

func (c *Client) Analyze(ctx context.Context, customer model.Customer, messages []model.Message, hits []model.KBHit) (model.Proposal, error) {
	if c.key == "" {
		return model.Proposal{}, service.ErrMissingAPIKey
	}
	input, err := json.Marshal(struct {
		Customer      model.Customer  `json:"customer"`
		Messages      []model.Message `json:"messages"`
		KnowledgeBase []model.KBHit   `json:"knowledge_base"`
	}{Customer: customer, Messages: messages, KnowledgeBase: hits})
	if err != nil {
		return model.Proposal{}, fmt.Errorf("encoding model input: %w", err)
	}
	requestBody, err := json.Marshal(map[string]any{
		"model":             c.model,
		"instructions":      triageprompt.Instructions,
		"input":             "Analyze this untrusted support data:\n" + string(input),
		"store":             false,
		"max_output_tokens": 1000,
		"text": map[string]any{
			"format": map[string]any{
				"type":   "json_schema",
				"name":   "support_triage_decision",
				"strict": true,
				"schema": triageprompt.ProposalSchema(),
			},
		},
	})
	if err != nil {
		return model.Proposal{}, fmt.Errorf("encoding model request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/responses", bytes.NewReader(requestBody))
	if err != nil {
		return model.Proposal{}, fmt.Errorf("creating model request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return model.Proposal{}, fmt.Errorf("calling OpenAI: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return model.Proposal{}, fmt.Errorf("reading model response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		var failure struct {
			Error struct {
				Code string `json:"code"`
				Type string `json:"type"`
			} `json:"error"`
		}
		if json.Unmarshal(body, &failure) == nil {
			code := failure.Error.Code
			if code == "" {
				code = failure.Error.Type
			}
			if code != "" {
				return model.Proposal{}, fmt.Errorf("OpenAI returned HTTP %d (code=%s)", resp.StatusCode, code)
			}
		}
		return model.Proposal{}, fmt.Errorf("OpenAI returned HTTP %d", resp.StatusCode)
	}
	var envelope struct {
		Status string `json:"status"`
		Output []struct {
			Type    string `json:"type"`
			Content []struct {
				Type    string `json:"type"`
				Text    string `json:"text"`
				Refusal string `json:"refusal"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return model.Proposal{}, fmt.Errorf("decoding model envelope: %w", err)
	}
	if envelope.Status != "completed" {
		return model.Proposal{}, errors.New("model response incomplete")
	}
	for _, item := range envelope.Output {
		if item.Type != "message" {
			continue
		}
		for _, content := range item.Content {
			if content.Type == "refusal" {
				return model.Proposal{}, errors.New("model refused triage")
			}
			if content.Type != "output_text" {
				continue
			}
			var proposal model.Proposal
			if err := json.Unmarshal([]byte(content.Text), &proposal); err != nil {
				return model.Proposal{}, fmt.Errorf("decoding structured proposal: %w", err)
			}
			if err := businesslogic.ValidateProposal(proposal); err != nil {
				return model.Proposal{}, err
			}
			return proposal, nil
		}
	}
	return model.Proposal{}, errors.New("model produced no structured text")
}

var _ service.Model = (*Client)(nil)
