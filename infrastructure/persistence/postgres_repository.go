package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/leo-andrei/check-in-service/domain/entities"
	"github.com/leo-andrei/check-in-service/domain/events"
	"github.com/leo-andrei/check-in-service/domain/repositories"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

type PostgresTimeRecordRepository struct {
	db *sql.DB
}

func NewPostgresTimeRecordRepository(db *sql.DB) *PostgresTimeRecordRepository {
	return &PostgresTimeRecordRepository{db: db}
}

func (r *PostgresTimeRecordRepository) Save(ctx context.Context, record *entities.TimeRecord) error {
	query := `
		INSERT INTO time_records (id, employee_id, check_in_at, check_out_at, status, hours_worked)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO UPDATE SET
			check_out_at = EXCLUDED.check_out_at,
			status = EXCLUDED.status,
			hours_worked = EXCLUDED.hours_worked
	`

	_, err := r.db.ExecContext(ctx, query,
		record.ID,
		record.EmployeeID,
		record.CheckInAt,
		record.CheckOutAt,
		record.Status,
		record.HoursWorked,
	)

	if err != nil {
		return fmt.Errorf("failed to save time record: %w", err)
	}

	return nil
}

// SaveWithEvent - Transactional Outbox Pattern Implementation
func (r *PostgresTimeRecordRepository) SaveWithEvent(ctx context.Context, record *entities.TimeRecord, event events.DomainEvent) error {
	// Start transaction
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() // Rollback if not committed

	// 1. Save the time record
	query := `
		INSERT INTO time_records (id, employee_id, check_in_at, check_out_at, status, hours_worked)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO UPDATE SET
			check_out_at = EXCLUDED.check_out_at,
			status = EXCLUDED.status,
			hours_worked = EXCLUDED.hours_worked,
			updated_at = CURRENT_TIMESTAMP
	`

	_, err = tx.ExecContext(ctx, query,
		record.ID,
		record.EmployeeID,
		record.CheckInAt,
		record.CheckOutAt,
		record.Status,
		record.HoursWorked,
	)

	if err != nil {
		return fmt.Errorf("failed to save time record: %w", err)
	}

	// 2. Save the event to outbox table (same transaction)
	eventPayload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	outboxQuery := `
		INSERT INTO outbox_events (id, event_type, aggregate_id, payload, created_at, published)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	_, err = tx.ExecContext(ctx, outboxQuery,
		uuid.New().String(),
		event.EventType(),
		record.ID,
		eventPayload,
		time.Now(),
		false,
	)

	if err != nil {
		return fmt.Errorf("failed to save outbox event: %w", err)
	}

	// 3. Commit transaction - both or neither
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

func (r *PostgresTimeRecordRepository) FindActiveByEmployeeID(ctx context.Context, employeeID string) (*entities.TimeRecord, error) {
	query := `
		SELECT id, employee_id, check_in_at, check_out_at, status, hours_worked
		FROM time_records
		WHERE employee_id = $1 AND status = $2
		ORDER BY check_in_at DESC
		LIMIT 1
	`

	var record entities.TimeRecord
	err := r.db.QueryRowContext(ctx, query, employeeID, entities.StatusCheckedIn).Scan(
		&record.ID,
		&record.EmployeeID,
		&record.CheckInAt,
		&record.CheckOutAt,
		&record.Status,
		&record.HoursWorked,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("failed to find active record: %w", err)
	}

	return &record, nil
}

func (r *PostgresTimeRecordRepository) FindByID(ctx context.Context, id string) (*entities.TimeRecord, error) {
	query := `
		SELECT id, employee_id, check_in_at, check_out_at, status, hours_worked
		FROM time_records
		WHERE id = $1
	`

	var record entities.TimeRecord
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&record.ID,
		&record.EmployeeID,
		&record.CheckInAt,
		&record.CheckOutAt,
		&record.Status,
		&record.HoursWorked,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("record not found")
	}

	if err != nil {
		return nil, fmt.Errorf("failed to find record: %w", err)
	}

	return &record, nil
}

// Outbox Repository Implementation
type PostgresOutboxRepository struct {
	db *sql.DB
}

func NewPostgresOutboxRepository(db *sql.DB) *PostgresOutboxRepository {
	return &PostgresOutboxRepository{db: db}
}

func (r *PostgresOutboxRepository) GetUnpublishedEvents(ctx context.Context, limit int) ([]repositories.OutboxEvent, error) {
	query := `
		SELECT id, event_type, aggregate_id, payload, created_at, published, retry_count
		FROM outbox_events
		WHERE published = FALSE AND event_type = $1
		ORDER BY created_at ASC
		LIMIT $2
		FOR UPDATE SKIP LOCKED
	`

	rows, err := r.db.QueryContext(ctx, query, events.EventTypeEmployeeCheckedOut, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query unpublished events: %w", err)
	}
	defer rows.Close()

	var events []repositories.OutboxEvent
	for rows.Next() {
		var event repositories.OutboxEvent
		err := rows.Scan(
			&event.ID,
			&event.EventType,
			&event.AggregateID,
			&event.Payload,
			&event.CreatedAt,
			&event.Published,
			&event.RetryCount,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan event: %w", err)
		}
		events = append(events, event)
	}

	return events, nil
}

func (r *PostgresOutboxRepository) MarkAsPublished(ctx context.Context, eventID string) error {
	query := `
		UPDATE outbox_events
		SET published = TRUE, published_at = $1
		WHERE id = $2
	`

	_, err := r.db.ExecContext(ctx, query, time.Now(), eventID)
	if err != nil {
		return fmt.Errorf("failed to mark event as published: %w", err)
	}

	return nil
}

func (r *PostgresOutboxRepository) IncrementRetryCount(ctx context.Context, eventID string, errorMsg string) error {
	query := `
		UPDATE outbox_events
		SET retry_count = retry_count + 1, last_error = $1
		WHERE id = $2
	`

	_, err := r.db.ExecContext(ctx, query, errorMsg, eventID)
	if err != nil {
		return fmt.Errorf("failed to increment retry count: %w", err)
	}

	return nil
}
