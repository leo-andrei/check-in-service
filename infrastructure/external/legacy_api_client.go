package external

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/leo-andrei/check-in-service/infrastructure/config"
	"go.uber.org/zap"
)

type LegacyLaborCostClient struct {
	baseURL        string
	httpClient     *http.Client
	circuitBreaker *CircuitBreaker
}

func NewLegacyLaborCostClient(baseURL string, cb *CircuitBreaker) *LegacyLaborCostClient {
	timeoutSec := 30
	if v, ok := interface{}(cb).(interface{ TimeoutSec() int }); ok {
		timeoutSec = v.TimeoutSec()
	}
	if config.Cfg.LegacyAPI.TimeoutSec > 0 {
		timeoutSec = config.Cfg.LegacyAPI.TimeoutSec
	}
	return &LegacyLaborCostClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: time.Duration(timeoutSec) * time.Second,
		},
		circuitBreaker: cb,
	}
}

type LaborCostRequest struct {
	EmployeeID  string  `json:"employee_id"`
	HoursWorked float64 `json:"hours_worked"`
	RecordedAt  string  `json:"recorded_at"`
}

func (c *LegacyLaborCostClient) RecordLaborCost(ctx context.Context, employeeID string, hours float64) error {
	// Log request
	config.Logger.Info("Sending labor cost to legacy API", zap.String("employee_id", employeeID), zap.Float64("hours", hours))
	if c.circuitBreaker != nil {
		canExecute, err := c.circuitBreaker.CanExecute()
		if err != nil {
			return fmt.Errorf("circuit breaker error: %w", err)
		}
		if !canExecute {
			return fmt.Errorf("circuit breaker open: legacy API temporarily unavailable")
		}
	}

	reqBody := LaborCostRequest{
		EmployeeID:  employeeID,
		HoursWorked: hours,
		RecordedAt:  time.Now().Format(time.RFC3339),
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		config.Logger.Error("Failed to marshal labor cost request", zap.Error(err))
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/labor-cost", bytes.NewBuffer(jsonBody))
	if err != nil {
		config.Logger.Error("Failed to create labor cost request", zap.Error(err))
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if c.circuitBreaker != nil {
			c.circuitBreaker.RecordFailure()
		}
		config.Logger.Error("Failed to send labor cost request", zap.Error(err))
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		if c.circuitBreaker != nil {
			c.circuitBreaker.RecordFailure()
		}
		config.Logger.Error("Unexpected status code from legacy API", zap.Int("status_code", resp.StatusCode))
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	config.Logger.Info("Labor cost sent successfully", zap.String("employee_id", employeeID), zap.Float64("hours", hours))

	if c.circuitBreaker != nil {
		c.circuitBreaker.RecordSuccess()
	}

	return nil
}
