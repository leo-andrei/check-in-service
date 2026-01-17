package services

import (
	"context"
	"fmt"
	"time"

	"github.com/leo-andrei/check-in-service/infrastructure/config"
	"go.uber.org/zap"

	"github.com/google/uuid"

	"github.com/leo-andrei/check-in-service/domain/entities"
	"github.com/leo-andrei/check-in-service/domain/errors"
	"github.com/leo-andrei/check-in-service/domain/events"
	"github.com/leo-andrei/check-in-service/domain/repositories"
)

type EventPublisher interface {
	Publish(ctx context.Context, event events.DomainEvent) error
}

type CheckInService struct {
	repo      repositories.TimeRecordRepository
	publisher EventPublisher
}

func NewCheckInService(repo repositories.TimeRecordRepository, publisher EventPublisher) *CheckInService {
	return &CheckInService{
		repo:      repo,
		publisher: publisher,
	}
}

func (s *CheckInService) CheckIn(ctx context.Context, employeeID string) (*entities.TimeRecord, error) {
	// Check if already checked in
	existing, err := s.repo.FindActiveByEmployeeID(ctx, employeeID)
	if err == nil && existing != nil {
		config.Logger.Warn(errors.ErrEmployeeAlreadyCheckedIn, zap.String("employee_id", employeeID))
		return nil, errors.ErrEmployeeAlreadyCheckedInConst
	}

	// Create new time record
	record, err := entities.NewTimeRecord(employeeID)
	if err != nil {
		config.Logger.Error("Failed to create time record", zap.String("employee_id", employeeID), zap.Error(err))
		return nil, err
	}

	// Create event
	event := events.EmployeeCheckedInEvent{
		EventHeader: events.EventHeader{
			EventID:   uuid.New().String(),
			EventType: events.EventTypeEmployeeCheckedIn,
			Version:   1, // Current schema version
			Timestamp: time.Now(),
		},
		EmployeeID: record.EmployeeID,
		CheckInAt:  record.CheckInAt,
		RecordID:   record.ID,
	}

	// Save to database with event in single transaction (Transactional Outbox)
	if err := s.repo.SaveWithEvent(ctx, record, event); err != nil {
		config.Logger.Error("Failed to save check-in", zap.String("employee_id", employeeID), zap.Error(err))
		return nil, fmt.Errorf("failed to save check-in: %w", err)
	}

	config.Logger.Info("Check-in successful", zap.String("employee_id", employeeID), zap.String("record_id", record.ID))

	// Event is now safely stored in outbox table
	// Outbox publisher will handle publishing to RabbitMQ

	return record, nil
}

type CheckOutService struct {
	repo      repositories.TimeRecordRepository
	publisher EventPublisher
}

func NewCheckOutService(repo repositories.TimeRecordRepository, publisher EventPublisher) *CheckOutService {
	return &CheckOutService{
		repo:      repo,
		publisher: publisher,
	}
}

func (s *CheckOutService) CheckOut(ctx context.Context, employeeID string) (*entities.TimeRecord, error) {
	// Find active check-in
	record, err := s.repo.FindActiveByEmployeeID(ctx, employeeID)
	if err != nil {
		config.Logger.Info(errors.ErrNoActiveCheckInFound, zap.String("employee_id", employeeID), zap.Error(err))
		return nil, errors.ErrNoActiveCheckInFoundConst
	}

	// Check if record is nil
	if record == nil {
		config.Logger.Info(errors.ErrNoActiveCheckInFound, zap.String("employee_id", employeeID))
		return nil, errors.ErrNoActiveCheckInFoundConst
	}

	// Check if it's a duplicate request - an user might double tap the card reader by mistake (window configurable)
	dupWindow := config.Cfg.CheckOut.DuplicateWindowSec
	if time.Since(record.CheckInAt) < time.Duration(dupWindow)*time.Second {
		config.Logger.Warn(errors.ErrDuplicateCheckIn, zap.String("employee_id", employeeID), zap.String("record_id", record.ID))
		return nil, errors.ErrDuplicateCheckInConst
	}

	// Execute check-out
	if err := record.CheckOut(); err != nil {
		config.Logger.Error("Failed to check out", zap.String("employee_id", employeeID), zap.String("record_id", record.ID), zap.Error(err))
		return nil, err
	}

	// Create event (this triggers labor cost reporting and email)
	event := events.EmployeeCheckedOutEvent{
		EventHeader: events.EventHeader{
			EventID:   uuid.New().String(),
			EventType: events.EventTypeEmployeeCheckedOut,
			Version:   1, // Current schema version
			Timestamp: time.Now(),
		},
		EmployeeID:  record.EmployeeID,
		CheckInAt:   record.CheckInAt,
		CheckOutAt:  *record.CheckOutAt,
		HoursWorked: record.HoursWorked,
		RecordID:    record.ID,
	}

	// Save to database with event in single transaction (Transactional Outbox)
	if err := s.repo.SaveWithEvent(ctx, record, event); err != nil {
		config.Logger.Error("Failed to save check-out", zap.String("employee_id", employeeID), zap.String("record_id", record.ID), zap.Error(err))
		return nil, fmt.Errorf("failed to save check-out: %w", err)
	}

	config.Logger.Info("Check-out successful", zap.String("employee_id", employeeID), zap.String("record_id", record.ID))

	// Event is now safely stored in outbox table
	// Outbox publisher will handle publishing to RabbitMQ

	return record, nil
}
