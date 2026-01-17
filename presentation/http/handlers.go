package http

import (
	"encoding/json"
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/leo-andrei/check-in-service/application/services"
	"github.com/leo-andrei/check-in-service/domain/errors"
)

type CheckInHandler struct {
	checkInService  *services.CheckInService
	checkOutService *services.CheckOutService
}

func NewCheckInHandler(
	checkInService *services.CheckInService,
	checkOutService *services.CheckOutService,
) *CheckInHandler {
	return &CheckInHandler{
		checkInService:  checkInService,
		checkOutService: checkOutService,
	}
}

type CheckInRequest struct {
	EmployeeID string `json:"employee_id" validate:"required,min=3,max=50,alphanum"`
}

func validateRequest(req *CheckInRequest) error {
	validate := validator.New()
	return validate.Struct(req)
}

type CheckInResponse struct {
	Success     bool    `json:"success"`
	Message     string  `json:"message"`
	RecordID    string  `json:"record_id,omitempty"`
	Action      string  `json:"action"` // "checked_in" or "checked_out"
	HoursWorked float64 `json:"hours_worked,omitempty"`
}

func (h *CheckInHandler) HandleCheckIn(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, errors.ErrMethodNotAllowed, http.StatusMethodNotAllowed)
		return
	}

	var req CheckInRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, errors.ErrInvalidRequestBody, http.StatusBadRequest)
		return
	}

	if req.EmployeeID == "" {
		http.Error(w, errors.ErrInvalidEmployeeID, http.StatusBadRequest)
		return
	}

	if err := validateRequest(&req); err != nil {
		http.Error(w, errors.ErrInvalidRequest, http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Try to check out first (if already checked in)
	record, err := h.checkOutService.CheckOut(ctx, req.EmployeeID)
	if err == nil {
		// Successfully checked out
		resp := CheckInResponse{
			Success:     true,
			Message:     "Successfully checked out",
			RecordID:    record.ID,
			Action:      "checked_out",
			HoursWorked: record.HoursWorked,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}

	// Not checked out, so check in
	record, err = h.checkInService.CheckIn(ctx, req.EmployeeID)
	if err != nil {
		if err == errors.ErrEmployeeAlreadyCheckedInConst {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := CheckInResponse{
		Success:  true,
		Message:  "Successfully checked in",
		RecordID: record.ID,
		Action:   "checked_in",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *CheckInHandler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}
