package entities

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type TimeRecordStatus string

const (
	StatusCheckedIn  TimeRecordStatus = "CHECKED_IN"
	StatusCheckedOut TimeRecordStatus = "CHECKED_OUT"
)

type TimeRecord struct {
	ID          string
	EmployeeID  string
	CheckInAt   time.Time
	CheckOutAt  *time.Time
	Status      TimeRecordStatus
	HoursWorked float64
}

func NewTimeRecord(employeeID string) (*TimeRecord, error) {
	if employeeID == "" {
		return nil, errors.New("employee ID cannot be empty")
	}

	return &TimeRecord{
		ID:         uuid.New().String(),
		EmployeeID: employeeID,
		CheckInAt:  time.Now(),
		Status:     StatusCheckedIn,
	}, nil
}

func (tr *TimeRecord) CheckOut() error {
	if tr.Status == StatusCheckedOut {
		return errors.New("already checked out")
	}

	now := time.Now()
	tr.CheckOutAt = &now
	tr.Status = StatusCheckedOut
	tr.HoursWorked = now.Sub(tr.CheckInAt).Hours()

	return nil
}

func (tr *TimeRecord) IsCheckedIn() bool {
	return tr.Status == StatusCheckedIn
}
