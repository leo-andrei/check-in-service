package errors

import "errors"

const (
	ErrFailedToSaveCheckIn      = "failed to save check-in"
	ErrFailedToSaveCheckOut     = "failed to save check-out"
	ErrInvalidEmployeeID        = "employee_id is required"
	ErrInvalidRequestBody       = "invalid request body"
	ErrInvalidRequest           = "invalid request"
	ErrMethodNotAllowed         = "method not allowed"
	ErrNoActiveCheckInFound     = "no active check-in found for employee"
	ErrEmployeeAlreadyCheckedIn = "employee is already checked in"
	ErrDuplicateCheckIn         = "duplicate check-in request (already checked in within 60 seconds)"
)

var (
	ErrEmployeeAlreadyCheckedInConst = errors.New(ErrEmployeeAlreadyCheckedIn)
	ErrDuplicateCheckInConst         = errors.New(ErrDuplicateCheckIn)
	ErrNoActiveCheckInFoundConst     = errors.New(ErrNoActiveCheckInFound)
)
