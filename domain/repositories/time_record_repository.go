package repositories

import (
	"context"
	"time"

	"github.com/leo-andrei/check-in-service/domain/entities"
	"github.com/leo-andrei/check-in-service/domain/events"
)

type TimeRecordRepository interface {
	Save(ctx context.Context, record *entities.TimeRecord) error
	SaveWithEvent(ctx context.Context, record *entities.TimeRecord, event events.DomainEvent) error
	FindActiveByEmployeeID(ctx context.Context, employeeID string) (*entities.TimeRecord, error)
	FindByID(ctx context.Context, id string) (*entities.TimeRecord, error)
}

type OutboxRepository interface {
	SaveEvent(ctx context.Context, event events.DomainEvent) error
	GetUnpublishedEvents(ctx context.Context, limit int) ([]OutboxEvent, error)
	MarkAsPublished(ctx context.Context, eventID string) error
	IncrementRetryCount(ctx context.Context, eventID string, errorMsg string) error
}

type OutboxEvent struct {
	ID          string
	EventType   string
	AggregateID string
	Payload     []byte
	CreatedAt   time.Time
	Published   bool
	RetryCount  int
}
