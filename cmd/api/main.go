package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/leo-andrei/check-in-service/application/handlers"
	"github.com/leo-andrei/check-in-service/application/services"
	"github.com/leo-andrei/check-in-service/infrastructure/config"
	"github.com/leo-andrei/check-in-service/infrastructure/external"
	"github.com/leo-andrei/check-in-service/infrastructure/messaging"
	"github.com/leo-andrei/check-in-service/infrastructure/persistence"
	httphandlers "github.com/leo-andrei/check-in-service/presentation/http"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"

	_ "github.com/lib/pq"
)

func main() {
	// Initialize zap logger
	logger, err := config.InitLogger()
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Sync()

	// Initialize OpenTelemetry
	ctx := context.Background()
	tp, err := config.InitTracerProvider(ctx, "check-in-service")
	if err != nil {
		logger.Fatal("Failed to initialize OpenTelemetry", zap.Error(err))
	}
	defer func() {
		_ = tp.Shutdown(ctx)
	}()

	// Load centralized config
	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	dbConnStr := cfg.Database.URL
	rabbitURL := cfg.RabbitMQ.URL
	legacyAPIURL := cfg.LegacyAPI.URL
	smtpHost := cfg.SMTP.Host

	// Initialize database
	db, err := sql.Open("postgres", dbConnStr)
	if err != nil {
		logger.Fatal("Failed to connect to database", zap.Error(err))
	}
	defer db.Close()

	// Create tables
	if err := initDatabase(db); err != nil {
		logger.Fatal("Failed to initialize database", zap.Error(err))
	}

	// Initialize repositories
	timeRecordRepo := persistence.NewPostgresTimeRecordRepository(db)
	outboxRepo := persistence.NewPostgresOutboxRepository(db)

	// Initialize event publisher
	publisher, err := messaging.NewRabbitMQPublisher(rabbitURL, "checkout-events")
	if err != nil {
		logger.Fatal("Failed to create publisher", zap.Error(err))
	}
	defer publisher.Close()

	// Initialize application services
	checkInService := services.NewCheckInService(timeRecordRepo, publisher)
	checkOutService := services.NewCheckOutService(timeRecordRepo, publisher)

	// Initialize HTTP handlers
	checkInHandler := httphandlers.NewCheckInHandler(checkInService, checkOutService)

	// Setup HTTP routes
	mux := http.NewServeMux()
	mux.HandleFunc("/api/checkin", checkInHandler.HandleCheckIn)
	mux.HandleFunc("/health", checkInHandler.HealthCheck)

	// Start HTTP server with configurable port
	httpPort := cfg.Server.Port
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", httpPort),
		Handler: mux,
	}

	       go func() {
		       logger.Info("Starting HTTP server", zap.String("port", fmt.Sprintf("%d", httpPort)))
		       if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			       logger.Fatal("HTTP server error", zap.Error(err))
		       }
	       }()

	// Start workers (consumers)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start Outbox Publisher (polls outbox and publishes to RabbitMQ)
	go startOutboxPublisher(ctx, outboxRepo, publisher)

	// Labor cost worker
	go startLaborCostWorker(ctx, rabbitURL, legacyAPIURL)

	// Email worker
	go startEmailWorker(ctx, rabbitURL, smtpHost)

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down gracefully...")

	// Shutdown HTTP server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("Server shutdown error", zap.Error(err))
	}

	logger.Info("Server stopped")

	// Cancel workers
	cancel()

	// Give some time for goroutines to finish
	time.Sleep(2 * time.Second)
	logger.Info("Application exited")

}

func startOutboxPublisher(ctx context.Context, outboxRepo *persistence.PostgresOutboxRepository, publisher *messaging.RabbitMQPublisher) {
	pollInterval := config.Cfg.Outbox.PollIntervalSec
	ticker := time.NewTicker(time.Duration(pollInterval) * time.Second)
	defer ticker.Stop()

	config.Logger.Info("Outbox publisher started")

	for {
		select {
		case <-ctx.Done():
			config.Logger.Info("Outbox publisher shutting down")
			return

		case <-ticker.C:
			// Start a new OpenTelemetry span for each poll cycle
			tracer := otel.Tracer("check-in-service")
			pollCtx, span := tracer.Start(ctx, "OutboxPublisherPoll")
			defer span.End()

			// Fetch unpublished events
			maxEvents := config.Cfg.Outbox.FetchLimit
			events, err := outboxRepo.GetUnpublishedEvents(pollCtx, maxEvents)
			if err != nil {
				config.Logger.Error("Error fetching unpublished events", zap.Error(err))
				span.RecordError(err)
				continue
			}

			if len(events) == 0 {
				span.AddEvent("No unpublished events found")
				continue
			}

			config.Logger.Info("Publishing events from outbox", zap.Int("count", len(events)))
			span.SetAttributes()

			for _, event := range events {
				// Try to publish to RabbitMQ
				err := publisher.PublishRaw(pollCtx, event.EventType, event.Payload)
				if err != nil {
					config.Logger.Error("Failed to publish event", zap.String("event_id", event.ID), zap.Error(err))
					span.RecordError(err)
					// Increment retry count
					outboxRepo.IncrementRetryCount(pollCtx, event.ID, err.Error())
					continue
				}

				// Successfully published - mark as published
				err = outboxRepo.MarkAsPublished(pollCtx, event.ID)
				if err != nil {
					config.Logger.Error("Failed to mark event as published", zap.String("event_id", event.ID), zap.Error(err))
					span.RecordError(err)
					continue
				}

				config.Logger.Info("Successfully published event", zap.String("event_id", event.ID), zap.String("type", event.EventType))
				span.AddEvent("Published event") // You can add attributes here if you want

			}
		}
	}
}

func startLaborCostWorker(ctx context.Context, rabbitURL, legacyAPIURL string) {
	consumer, err := messaging.NewRabbitMQConsumer(rabbitURL, "checkout-events", "labor-cost-queue")
	if err != nil {
		log.Fatalf("Failed to create labor cost consumer: %v", err)
	}
	defer consumer.Close()
	cbFailures := config.Cfg.CircuitBreaker.MaxFailures
	cbReset := config.Cfg.CircuitBreaker.ResetTimeoutS
	cb := external.NewCircuitBreaker(cbFailures, 1, time.Duration(cbReset)*time.Second)
	legacyClient := external.NewLegacyLaborCostClient(legacyAPIURL, cb)
	handler := handlers.NewLaborCostReporter(legacyClient)

	config.Logger.Info("Labor cost worker started")
	if err := consumer.Consume(ctx, handler.HandleCheckedOut); err != nil {
		config.Logger.Error("Labor cost consumer error", zap.Error(err))
	}
}

func startEmailWorker(ctx context.Context, rabbitURL, smtpHost string) {
	consumer, err := messaging.NewRabbitMQConsumer(rabbitURL, "checkout-events", "email-queue")
	if err != nil {
		log.Fatalf("Failed to create email consumer: %v", err)
	}
	defer consumer.Close()

	smtpPort := config.Cfg.SMTP.Port
	emailClient := external.NewEmailClient(smtpHost, smtpPort)
	handler := handlers.NewEmailNotifier(emailClient)

	config.Logger.Info("Email worker started")
	if err := consumer.Consume(ctx, handler.HandleCheckedOut); err != nil {
		config.Logger.Error("Email consumer error", zap.Error(err))
	}
}

func initDatabase(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS time_records (
		id VARCHAR(255) PRIMARY KEY,
		employee_id VARCHAR(255) NOT NULL,
		check_in_at TIMESTAMP NOT NULL,
		check_out_at TIMESTAMP,
		status VARCHAR(50) NOT NULL,
		hours_worked DECIMAL(10, 2) DEFAULT 0,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_employee_status ON time_records(employee_id, status);

	-- Outbox pattern table for guaranteed event delivery
	CREATE TABLE IF NOT EXISTS outbox_events (
		id VARCHAR(255) PRIMARY KEY,
		event_type VARCHAR(100) NOT NULL,
		aggregate_id VARCHAR(255) NOT NULL,
		payload JSONB NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		published BOOLEAN DEFAULT FALSE,
		published_at TIMESTAMP,
		retry_count INT DEFAULT 0,
		last_error TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_outbox_unpublished ON outbox_events(published, created_at) WHERE published = FALSE;
	`

	_, err := db.Exec(schema)
	return err
}
