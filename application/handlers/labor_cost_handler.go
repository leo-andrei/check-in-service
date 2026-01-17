package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/leo-andrei/check-in-service/domain/events"
	"github.com/leo-andrei/check-in-service/infrastructure/external"
)

type LaborCostReporter struct {
	legacyClient *external.LegacyLaborCostClient
	retryConfig  RetryConfig
}

type RetryConfig struct {
	MaxAttempts       int
	InitialBackoff    time.Duration
	MaxBackoff        time.Duration
	BackoffMultiplier float64
}

func NewLaborCostReporter(client *external.LegacyLaborCostClient) *LaborCostReporter {
	return &LaborCostReporter{
		legacyClient: client,
		retryConfig: RetryConfig{
			MaxAttempts:       5,
			InitialBackoff:    1 * time.Second,
			MaxBackoff:        30 * time.Second,
			BackoffMultiplier: 2.0,
		},
	}
}

func (h *LaborCostReporter) HandleCheckedOut(ctx context.Context, eventData []byte) error {
	var event events.EmployeeCheckedOutEvent
	if err := json.Unmarshal(eventData, &event); err != nil {
		return fmt.Errorf("failed to unmarshal event: %w", err)
	}

	// Retry logic with exponential backoff
	attempt := 0
	backoff := h.retryConfig.InitialBackoff

	for attempt < h.retryConfig.MaxAttempts {
		err := h.legacyClient.RecordLaborCost(ctx, event.EmployeeID, event.HoursWorked)
		if err == nil {
			return nil
		}

		attempt++
		if attempt >= h.retryConfig.MaxAttempts {
			return fmt.Errorf("failed after %d attempts: %w", attempt, err)
		}

		fmt.Printf("Retry %d/%d for employee %s after error: %v\n",
			attempt, h.retryConfig.MaxAttempts, event.EmployeeID, err)

		time.Sleep(backoff)
		backoff = time.Duration(float64(backoff) * h.retryConfig.BackoffMultiplier)
		if backoff > h.retryConfig.MaxBackoff {
			backoff = h.retryConfig.MaxBackoff
		}
	}

	return fmt.Errorf("max retries exceeded")
}
