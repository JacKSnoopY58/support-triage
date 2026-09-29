package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"support-triage-service/controller"
	"support-triage-service/providers/faq"
	"support-triage-service/providers/incident"
	"support-triage-service/providers/openai"
	"support-triage-service/repository/postgres"
	"support-triage-service/service"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		logger.Error("DATABASE_URL is required")
		os.Exit(1)
	}
	store, err := openStore(dsn)
	if err != nil {
		logger.Error("database unavailable", "error", err)
		os.Exit(1)
	}
	searcher := faq.New(delayFromEnv("FAQ_DELAY_MS"), os.Getenv("FAQ_FAIL") == "1")
	model := openai.New(os.Getenv("OPENAI_API_KEY"), "gpt-4o-mini", "", nil)
	gateway := incident.New(store, delayFromEnv("INCIDENT_DELAY_MS"), os.Getenv("INCIDENT_FAIL_MODE"))
	triageService := service.NewService(store, searcher, model, gateway, service.RealClock{}, logger)
	turnTimeout := 45 * time.Second
	api := controller.New(triageService, logger, turnTimeout)
	app := fiber.New(fiber.Config{
		BodyLimit:    1 << 20,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: turnTimeout + 10*time.Second,
	})
	api.Register(app)
	address := os.Getenv("HTTP_ADDR")
	if address == "" {
		address = ":8080"
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		if err := app.Shutdown(); err != nil {
			logger.Error("shutdown failed", "error", err)
		}
	}()
	logger.Info("server_start", "address", address, "model", model.Name(), "provider", "openai")
	if err := app.Listen(address); err != nil && ctx.Err() == nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func openStore(dsn string) (*postgres.Store, error) {
	var last error
	for range 20 {
		store, err := postgres.Open(dsn)
		if err == nil {
			return store, nil
		}
		last = err
		time.Sleep(time.Second)
	}
	return nil, last
}

func delayFromEnv(name string) time.Duration {
	milliseconds, err := strconv.Atoi(os.Getenv(name))
	if err != nil || milliseconds < 0 {
		return 0
	}
	return time.Duration(milliseconds) * time.Millisecond
}
