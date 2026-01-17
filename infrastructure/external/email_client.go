package external

import (
	"context"
	"fmt"
	"net/smtp"

	"github.com/leo-andrei/check-in-service/infrastructure/config"
	"go.uber.org/zap"
)

type EmailClient struct {
	smtpHost string
	smtpPort int
}

func NewEmailClient(smtpHost string, smtpPort int) *EmailClient {
	return &EmailClient{
		smtpHost: smtpHost,
		smtpPort: smtpPort,
	}
}

func (c *EmailClient) SendEmail(ctx context.Context, employeeID, subject, body string) error {
	config.Logger.Info("Sending email", zap.String("employee_id", employeeID), zap.String("subject", subject))

	// Connect to Mailhog SMTP server
	addr := fmt.Sprintf("%s:%d", c.smtpHost, c.smtpPort)
	err := smtp.SendMail(
		addr,
		nil, // no authentication for Mailhog
		"noreply@company.com",
		[]string{fmt.Sprintf("%s@company.com", employeeID)},
		[]byte(fmt.Sprintf("Subject: %s\r\n\r\n%s", subject, body)),
	)

	if err != nil {
		config.Logger.Error("Failed to send email", zap.String("employee_id", employeeID), zap.Error(err))
		return fmt.Errorf("failed to send email: %w", err)
	}

	config.Logger.Info("Email sent", zap.String("employee_id", employeeID), zap.String("subject", subject))
	return nil
}
