package notification

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/real-rm/goconfig"
	"github.com/real-rm/golog"
	"github.com/real-rm/gomail"
	"github.com/real-rm/gomongo"
	"github.com/real-rm/gosms"
)

// NotificationService handles sending email and SMS notifications
type NotificationService struct {
	mailer      *gomail.Mailer
	smsSender   *gosms.SMSSender
	logger      *golog.Logger
	config      *goconfig.ConfigAccessor
	rateLimiter *RateLimiter
	mu          sync.RWMutex
}

// RateLimiter prevents notification flooding
type RateLimiter struct {
	events map[string][]time.Time
	window time.Duration
	limit  int
	mu     sync.RWMutex
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(window time.Duration, limit int) *RateLimiter {
	return &RateLimiter{
		events: make(map[string][]time.Time),
		window: window,
		limit:  limit,
	}
}

// Allow checks if an event is allowed based on rate limiting
func (rl *RateLimiter) Allow(eventKey string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// Get existing events for this key
	events := rl.events[eventKey]

	// Filter out old events
	var recentEvents []time.Time
	for _, t := range events {
		if t.After(cutoff) {
			recentEvents = append(recentEvents, t)
		}
	}

	// Check if we're under the limit
	if len(recentEvents) >= rl.limit {
		return false
	}

	// Add this event
	recentEvents = append(recentEvents, now)
	rl.events[eventKey] = recentEvents

	return true
}

// NewNotificationService creates a new notification service
func NewNotificationService(
	logger *golog.Logger,
	config *goconfig.ConfigAccessor,
	mongo *gomongo.Mongo,
) (*NotificationService, error) {
	// Initialize gomail
	mailer, err := gomail.GetSendMailObj(gomail.MailerOptions{
		Logger: logger,
		Config: config,
		Mongo:  mongo, // Enable email logging
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize gomail: %w", err)
	}

	// Initialize gosms
	accountSID, err := config.ConfigString("sms.accountSID")
	if err != nil {
		logger.Warn("SMS account SID not configured", "error", err)
		accountSID = ""
	}

	authToken, err := config.ConfigString("sms.authToken")
	if err != nil {
		logger.Warn("SMS auth token not configured", "error", err)
		authToken = ""
	}

	var smsSender *gosms.SMSSender
	if accountSID != "" && authToken != "" {
		twilioEngine, err := gosms.NewTwilioEngine(accountSID, authToken)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize Twilio engine: %w", err)
		}

		smsSender, err = gosms.NewSMSSender(twilioEngine)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize SMS sender: %w", err)
		}
	} else {
		logger.Warn("SMS not configured - SMS notifications will be skipped")
	}

	// Create rate limiter: max 5 notifications per 5 minutes per event type
	rateLimiter := NewRateLimiter(5*time.Minute, 5)

	return &NotificationService{
		mailer:      mailer,
		smsSender:   smsSender,
		logger:      logger,
		config:      config,
		rateLimiter: rateLimiter,
	}, nil
}

// SendHelpRequestAlert sends notifications when a user requests help
func (ns *NotificationService) SendHelpRequestAlert(userID, sessionID string) error {
	eventKey := fmt.Sprintf("help_request:%s", sessionID)

	// Check rate limiting
	if !ns.rateLimiter.Allow(eventKey) {
		ns.logger.Warn("Help request notification rate limited", "session_id", sessionID)
		return nil // Don't return error, just skip
	}

	// Get admin emails and phones from config
	adminEmails, err := ns.getAdminEmails()
	if err != nil {
		return fmt.Errorf("failed to get admin emails: %w", err)
	}

	adminPhones, err := ns.getAdminPhones()
	if err != nil {
		return fmt.Errorf("failed to get admin phones: %w", err)
	}

	// Send email notification
	if len(adminEmails) > 0 {
		msg := &gomail.EmailMessage{
			To:      adminEmails,
			Subject: fmt.Sprintf("Help Request - User %s", userID),
			HTML: fmt.Sprintf(`
				<h2>User Help Request</h2>
				<p>A user has requested assistance.</p>
				<ul>
					<li><strong>User ID:</strong> %s</li>
					<li><strong>Session ID:</strong> %s</li>
					<li><strong>Time:</strong> %s</li>
				</ul>
				<p><a href="https://admin.example.com/sessions/%s">View Session</a></p>
			`, userID, sessionID, time.Now().Format(time.RFC3339), sessionID),
			Text: fmt.Sprintf("Help Request - User: %s, Session: %s, Time: %s",
				userID, sessionID, time.Now().Format(time.RFC3339)),
		}

		// Use SendWithRetry for automatic failover
		engines := ns.mailer.GetEngineNames()
		if err := ns.mailer.SendWithRetry(engines, msg); err != nil {
			ns.logger.Error("Failed to send help request email", "error", err, "session_id", sessionID)
			return fmt.Errorf("failed to send email: %w", err)
		}

		ns.logger.Info("Help request email sent", "session_id", sessionID, "recipients", len(adminEmails))
	}

	// Send SMS notification
	if ns.smsSender != nil && len(adminPhones) > 0 {
		fromNumber, err := ns.config.ConfigString("sms.fromNumber")
		if err != nil {
			ns.logger.Warn("SMS from number not configured", "error", err)
			fromNumber = ""
		}

		message := fmt.Sprintf("Help Request - User %s needs assistance. Session: %s", userID, sessionID)

		for _, phone := range adminPhones {
			opt := gosms.SendOption{
				To:      phone,
				From:    fromNumber,
				Message: message,
			}

			if err := ns.smsSender.Send(opt); err != nil {
				ns.logger.Error("Failed to send help request SMS", "error", err, "phone", phone)
				// Continue to next phone number
			} else {
				ns.logger.Info("Help request SMS sent", "phone", phone)
			}
		}
	}

	return nil
}

// SendCriticalError sends notifications for critical system errors
func (ns *NotificationService) SendCriticalError(errorType, details string, affectedUsers int) error {
	eventKey := fmt.Sprintf("critical_error:%s", errorType)

	// Check rate limiting
	if !ns.rateLimiter.Allow(eventKey) {
		ns.logger.Warn("Critical error notification rate limited", "error_type", errorType)
		return nil // Don't return error, just skip
	}

	// Get admin emails and phones from config
	adminEmails, err := ns.getAdminEmails()
	if err != nil {
		return fmt.Errorf("failed to get admin emails: %w", err)
	}

	adminPhones, err := ns.getAdminPhones()
	if err != nil {
		return fmt.Errorf("failed to get admin phones: %w", err)
	}

	// Send email notification
	if len(adminEmails) > 0 {
		msg := &gomail.EmailMessage{
			To:      adminEmails,
			Subject: fmt.Sprintf("CRITICAL: %s", errorType),
			HTML: fmt.Sprintf(`
				<h2 style="color: red;">Critical System Error</h2>
				<ul>
					<li><strong>Error Type:</strong> %s</li>
					<li><strong>Details:</strong> %s</li>
					<li><strong>Affected Users:</strong> %d</li>
					<li><strong>Time:</strong> %s</li>
				</ul>
				<p>Please investigate immediately.</p>
			`, errorType, details, affectedUsers, time.Now().Format(time.RFC3339)),
			Text: fmt.Sprintf("CRITICAL ERROR - Type: %s, Details: %s, Affected Users: %d, Time: %s",
				errorType, details, affectedUsers, time.Now().Format(time.RFC3339)),
		}

		// Use SendWithRetry for automatic failover - critical errors need high reliability
		engines := ns.mailer.GetEngineNames()
		if err := ns.mailer.SendWithRetry(engines, msg); err != nil {
			ns.logger.Error("Failed to send critical error email", "error", err, "error_type", errorType)
			return fmt.Errorf("failed to send email: %w", err)
		}

		ns.logger.Info("Critical error email sent", "error_type", errorType, "recipients", len(adminEmails))
	}

	// Send SMS notification
	if ns.smsSender != nil && len(adminPhones) > 0 {
		fromNumber, err := ns.config.ConfigString("sms.fromNumber")
		if err != nil {
			ns.logger.Warn("SMS from number not configured", "error", err)
			fromNumber = ""
		}

		message := fmt.Sprintf("CRITICAL: %s - %d users affected. Check email for details.", errorType, affectedUsers)

		for _, phone := range adminPhones {
			opt := gosms.SendOption{
				To:      phone,
				From:    fromNumber,
				Message: message,
			}

			if err := ns.smsSender.Send(opt); err != nil {
				ns.logger.Error("Failed to send critical error SMS", "error", err, "phone", phone)
				// Continue to next phone number
			} else {
				ns.logger.Info("Critical error SMS sent", "phone", phone)
			}
		}
	}

	return nil
}

// SendSystemAlert sends a general system alert
func (ns *NotificationService) SendSystemAlert(subject, message string) error {
	eventKey := fmt.Sprintf("system_alert:%s", subject)

	// Check rate limiting
	if !ns.rateLimiter.Allow(eventKey) {
		ns.logger.Warn("System alert notification rate limited", "subject", subject)
		return nil
	}

	// Get admin emails from config
	adminEmails, err := ns.getAdminEmails()
	if err != nil {
		return fmt.Errorf("failed to get admin emails: %w", err)
	}

	if len(adminEmails) == 0 {
		ns.logger.Warn("No admin emails configured for system alert")
		return nil
	}

	msg := &gomail.EmailMessage{
		To:      adminEmails,
		Subject: subject,
		HTML:    fmt.Sprintf("<p>%s</p><p><em>Time: %s</em></p>", message, time.Now().Format(time.RFC3339)),
		Text:    fmt.Sprintf("%s\nTime: %s", message, time.Now().Format(time.RFC3339)),
	}

	// Use SendMail with first available engine for non-critical alerts
	engines := ns.mailer.GetEngineNames()
	if len(engines) == 0 {
		return fmt.Errorf("no mail engines configured")
	}

	if err := ns.mailer.SendMail(engines[0], msg); err != nil {
		ns.logger.Error("Failed to send system alert email", "error", err, "subject", subject)
		return fmt.Errorf("failed to send email: %w", err)
	}

	ns.logger.Info("System alert email sent", "subject", subject, "recipients", len(adminEmails))
	return nil
}

// getAdminEmails retrieves admin email addresses from config
func (ns *NotificationService) getAdminEmails() ([]string, error) {
	// Try to get from notification.adminEmails array
	adminEmailsStr, err := ns.config.ConfigString("notification.adminEmails")
	if err == nil && adminEmailsStr != "" {
		// Parse as comma-separated list
		emails := []string{}
		for _, email := range splitAndTrim(adminEmailsStr) {
			if email != "" {
				emails = append(emails, email)
			}
		}
		if len(emails) > 0 {
			return emails, nil
		}
	}

	// Fallback to mail.adminEmail
	adminEmail, err := ns.config.ConfigString("mail.adminEmail")
	if err != nil {
		return nil, err
	}

	if adminEmail == "" {
		return []string{}, nil
	}

	return []string{adminEmail}, nil
}

// getAdminPhones retrieves admin phone numbers from config
func (ns *NotificationService) getAdminPhones() ([]string, error) {
	adminPhonesStr, err := ns.config.ConfigString("notification.adminPhones")
	if err != nil {
		// Not configured is okay
		return []string{}, nil
	}

	if adminPhonesStr == "" {
		return []string{}, nil
	}

	// Parse as comma-separated list
	phones := []string{}
	for _, phone := range splitAndTrim(adminPhonesStr) {
		if phone != "" {
			phones = append(phones, phone)
		}
	}

	return phones, nil
}

// splitAndTrim splits a string by comma and trims whitespace
func splitAndTrim(s string) []string {
	if s == "" {
		return []string{}
	}

	parts := []string{}
	for i := 0; i < len(s); {
		// Find next comma
		end := i
		for end < len(s) && s[end] != ',' {
			end++
		}

		// Extract and trim
		part := s[i:end]
		// Manual trim
		start := 0
		for start < len(part) && (part[start] == ' ' || part[start] == '\t' || part[start] == '\n' || part[start] == '\r') {
			start++
		}
		trimEnd := len(part)
		for trimEnd > start && (part[trimEnd-1] == ' ' || part[trimEnd-1] == '\t' || part[trimEnd-1] == '\n' || part[trimEnd-1] == '\r') {
			trimEnd--
		}

		if trimEnd > start {
			parts = append(parts, part[start:trimEnd])
		}

		i = end + 1
	}

	return parts
}

// ValidateEmails validates a list of email addresses using gomail's validation
func (ns *NotificationService) ValidateEmails(ctx context.Context, emails []string) ([]string, error) {
	return ns.mailer.GetValidatedEmails(ctx, emails)
}
