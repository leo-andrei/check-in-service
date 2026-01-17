package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/leo-andrei/check-in-service/domain/events"
	"github.com/leo-andrei/check-in-service/infrastructure/external"
)

type EmailNotifier struct {
	emailClient *external.EmailClient
}

func NewEmailNotifier(client *external.EmailClient) *EmailNotifier {
	return &EmailNotifier{
		emailClient: client,
	}
}

func (h *EmailNotifier) HandleCheckedOut(ctx context.Context, eventData []byte) error {
	var event events.EmployeeCheckedOutEvent
	if err := json.Unmarshal(eventData, &event); err != nil {
		return fmt.Errorf("failed to unmarshal event: %w", err)
	}

	subject := "Your Work Hours Summary"
	body := fmt.Sprintf(`
		Hello,
		
		You have successfully checked out.
		
		Check-in time: %s
		Check-out time: %s
		Hours worked: %.2f
		
		Thank you!
	`, event.CheckInAt.Format(time.RFC822),
		event.CheckOutAt.Format(time.RFC822),
		event.HoursWorked)

	err := h.emailClient.SendEmail(ctx, event.EmployeeID, subject, body)
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	return nil
}
