package events

import (
	"time"
)

const (
	EventTypeEmployeeCheckedIn  = "EmployeeCheckedIn"
	EventTypeEmployeeCheckedOut = "EmployeeCheckedOut"
)

type DomainEvent interface {
	EventType() string
	OccurredAt() time.Time
	Version() int
}

// EventHeader contains common fields for all domain events
// This enables schema versioning and backward compatibility
type EventHeader struct {
	EventID   string    `json:"event_id"`
	EventType string    `json:"event_type"`
	Version   int       `json:"version"` // For schema evolution
	Timestamp time.Time `json:"timestamp"`
}

type EmployeeCheckedInEvent struct {
	EventHeader
	EmployeeID string    `json:"employee_id"`
	CheckInAt  time.Time `json:"check_in_at"`
	RecordID   string    `json:"record_id"`
}

func (e EmployeeCheckedInEvent) EventType() string {
	return EventTypeEmployeeCheckedIn
}

func (e EmployeeCheckedInEvent) OccurredAt() time.Time {
	return e.Timestamp
}

func (e EmployeeCheckedInEvent) Version() int {
	return e.EventHeader.Version
}

type EmployeeCheckedOutEvent struct {
	EventHeader
	EmployeeID  string    `json:"employee_id"`
	CheckInAt   time.Time `json:"check_in_at"`
	CheckOutAt  time.Time `json:"check_out_at"`
	HoursWorked float64   `json:"hours_worked"`
	RecordID    string    `json:"record_id"`
}

func (e EmployeeCheckedOutEvent) EventType() string {
	return EventTypeEmployeeCheckedOut
}

func (e EmployeeCheckedOutEvent) OccurredAt() time.Time {
	return e.Timestamp
}

func (e EmployeeCheckedOutEvent) Version() int {
	return e.EventHeader.Version
}
