package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"support-triage-service/model"
	"support-triage-service/service"
)

type Controller struct {
	useCase     service.TriageUseCase
	logger      *slog.Logger
	turnTimeout time.Duration
}

func New(useCase service.TriageUseCase, logger *slog.Logger, turnTimeout time.Duration) *Controller {
	return &Controller{useCase: useCase, logger: logger, turnTimeout: turnTimeout}
}

func (api *Controller) Register(app *fiber.App) {
	app.Use(func(c *fiber.Ctx) error {
		start := time.Now()
		requestID := uuid.NewString()
		c.Locals("request_id", requestID)
		c.Set("X-Request-ID", requestID)
		err := c.Next()
		api.logger.Info("http_request",
			"request_id", requestID,
			"method", c.Method(), "path", c.Path(),
			"status", c.Response().StatusCode(),
			"duration_ms", time.Since(start).Milliseconds())
		return err
	})
	app.Get("/healthz", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})
	apiInfo := func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"service": "support-triage-service",
			"status":  "ok",
			"routes": []string{
				"GET /healthz", "POST /tickets",
				"POST /conversations/:id/messages", "GET /conversations/:id",
			},
		})
	}
	app.Get("/", apiInfo)
	app.Get("/api", apiInfo)
	app.Post("/tickets", api.createTicket)
	app.Post("/conversations/:id/messages", api.addMessage)
	app.Get("/conversations/:id", api.getConversation)
}

type ticketRequest struct {
	Customer model.Customer         `json:"customer"`
	Messages []service.MessageDraft `json:"messages"`
}

type messageRequest struct {
	Body       string    `json:"body"`
	OccurredAt time.Time `json:"occurred_at"`
}

func (api *Controller) createTicket(c *fiber.Ctx) error {
	key, ok := idempotencyKey(c)
	if !ok {
		return api.error(c, fiber.StatusUnprocessableEntity, "missing_idempotency_key", "Idempotency-Key header must be 8–128 characters")
	}
	var request ticketRequest
	if err := decodeBody(c.Body(), &request); err != nil {
		return api.error(c, fiber.StatusBadRequest, "invalid_json", err.Error())
	}
	if err := validateTicket(request); err != nil {
		return api.error(c, fiber.StatusUnprocessableEntity, "validation_error", err.Error())
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), api.turnTimeout)
	defer cancel()
	response, err := api.useCase.Ingest(ctx, key, service.TicketCommand{
		Customer: request.Customer, Messages: request.Messages,
	})
	if err != nil {
		return api.serviceError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(response)
}

func (api *Controller) addMessage(c *fiber.Ctx) error {
	key, ok := idempotencyKey(c)
	if !ok {
		return api.error(c, fiber.StatusUnprocessableEntity, "missing_idempotency_key", "Idempotency-Key header must be 8–128 characters")
	}
	var request messageRequest
	if err := decodeBody(c.Body(), &request); err != nil {
		return api.error(c, fiber.StatusBadRequest, "invalid_json", err.Error())
	}
	if strings.TrimSpace(request.Body) == "" || len(request.Body) > 5000 ||
		request.OccurredAt.IsZero() {
		return api.error(c, fiber.StatusUnprocessableEntity, "validation_error", "body must be 1–5000 characters and occurred_at must be RFC3339")
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), api.turnTimeout)
	defer cancel()
	response, err := api.useCase.Continue(ctx, c.Params("id"), key, service.TurnCommand{
		Body: request.Body, OccurredAt: request.OccurredAt,
	})
	if err != nil {
		return api.serviceError(c, err)
	}
	return c.JSON(response)
}

func (api *Controller) getConversation(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.UserContext(), 10*time.Second)
	defer cancel()
	conversation, err := api.useCase.Get(ctx, c.Params("id"))
	if err != nil {
		return api.serviceError(c, err)
	}
	return c.JSON(conversation)
}

func idempotencyKey(c *fiber.Ctx) (string, bool) {
	key := strings.TrimSpace(c.Get("Idempotency-Key"))
	return key, len(key) >= 8 && len(key) <= 128
}

func decodeBody(body []byte, target any) error {
	if len(body) == 0 {
		return errors.New("request body is empty")
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("malformed JSON: %w", err)
	}
	return nil
}

func validateTicket(request ticketRequest) error {
	if strings.TrimSpace(request.Customer.Plan) == "" || request.Customer.Seats < 1 ||
		request.Customer.TenureMonths < 0 || request.Customer.PriorTickets < 0 {
		return errors.New("customer plan and positive seats are required; counts cannot be negative")
	}
	if len(request.Messages) == 0 || len(request.Messages) > 100 {
		return errors.New("messages must contain 1–100 entries")
	}
	for i, message := range request.Messages {
		if message.Role != "customer" && message.Role != "operator" {
			return fmt.Errorf("messages[%d].role must be customer or operator", i)
		}
		if strings.TrimSpace(message.Body) == "" || len(message.Body) > 5000 {
			return fmt.Errorf("messages[%d].body must be 1–5000 characters", i)
		}
		if message.OccurredAt.IsZero() {
			return fmt.Errorf("messages[%d].occurred_at must be RFC3339", i)
		}
	}
	return nil
}

func (api *Controller) serviceError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrNotFound):
		return api.error(c, fiber.StatusNotFound, "not_found", "conversation not found")
	case errors.Is(err, service.ErrConflict):
		return api.error(c, fiber.StatusConflict, "idempotency_conflict", "key was already used with a different payload")
	case errors.Is(err, service.ErrInProgress):
		c.Set("Retry-After", "2")
		return api.error(c, fiber.StatusConflict, "request_in_progress", "request is already being processed")
	case errors.Is(err, service.ErrMissingAPIKey):
		return api.error(c, fiber.StatusServiceUnavailable, "model_not_configured", "OPENAI_API_KEY is not configured")
	case errors.Is(err, service.ErrModel):
		api.logger.Warn("model_request_failed", "request_id", c.Locals("request_id"), "error", err)
		return api.error(c, fiber.StatusBadGateway, "model_error", "could not obtain a valid model decision")
	default:
		api.logger.Error("request_failed", "request_id", c.Locals("request_id"), "error", err)
		return api.error(c, fiber.StatusServiceUnavailable, "service_unavailable", "request could not be completed")
	}
}

func (api *Controller) error(c *fiber.Ctx, status int, code, message string) error {
	return c.Status(status).JSON(fiber.Map{
		"error": fiber.Map{
			"code": code, "message": message,
			"request_id": c.Locals("request_id"),
		},
	})
}
