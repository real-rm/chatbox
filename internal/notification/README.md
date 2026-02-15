# Notification Service

The notification service handles sending email and SMS notifications to administrators using the `gomail` and `gosms` libraries.

## Features

- **Email notifications** via gomail with multi-engine support (SES, SMTP, Mock)
- **SMS notifications** via gosms with Twilio support
- **Rate limiting** to prevent notification flooding (5 notifications per 5 minutes per event type)
- **Automatic failover** for email using `SendWithRetry`
- **Email logging** to MongoDB (optional)

## Usage

### Initialization

```go
import (
    "github.com/real-rm/goconfig"
    "github.com/real-rm/golog"
    "github.com/real-rm/gomongo"
    "github.com/real-rm/chatbox/internal/notification"
)

// Initialize dependencies
logger, _ := golog.InitLog(golog.LogConfig{...})
config, _ := goconfig.Default()
mongo, _ := gomongo.InitMongoDB(logger, config)

// Create notification service
notificationService, err := notification.NewNotificationService(logger, config, mongo)
if err != nil {
    log.Fatal(err)
}
```

### Sending Help Request Alerts

```go
err := notificationService.SendHelpRequestAlert("user-123", "session-456")
if err != nil {
    log.Printf("Failed to send help request alert: %v", err)
}
```

### Sending Critical Error Alerts

```go
err := notificationService.SendCriticalError(
    "Database Connection Failed",
    "MongoDB connection timeout after 10s",
    5, // affected users
)
if err != nil {
    log.Printf("Failed to send critical error alert: %v", err)
}
```

### Sending System Alerts

```go
err := notificationService.SendSystemAlert(
    "High Memory Usage",
    "Memory usage exceeded 90% on server-01",
)
if err != nil {
    log.Printf("Failed to send system alert: %v", err)
}
```

## Configuration

The notification service requires the following configuration in `config.toml`:

### Email Configuration (gomail)

```toml
[mail]
defaultFromName = "Chat Support"
replyToEmail = "support@example.com"
adminEmail = "admin@example.com"

# SES Engine
[[mail.engines.ses]]
name = "ses-primary"
accessKeyId = "AKIAXXXXXXXX"
secretAccessKey = "xxxxxxxx"
region = "us-east-1"
from = "noreply@example.com"
fromName = "Chat System"

# SMTP Engine (backup)
[[mail.engines.smtp]]
name = "smtp-backup"
host = "smtp.example.com"
port = 587
user = "user@example.com"
pass = "password"
from = "noreply@example.com"

# Mock Engine (testing)
[[mail.engines.mock]]
name = "mock"
verbose = true
```

### SMS Configuration (gosms)

```toml
[sms]
provider = "twilio"
accountSID = "ACXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"
authToken = "your_auth_token"
fromNumber = "+1234567890"
```

### Notification Recipients

```toml
[notification]
adminEmails = ["admin@example.com"]
adminPhones = ["+1234567890"]
```

## Rate Limiting

The notification service implements rate limiting to prevent notification flooding:

- **Window**: 5 minutes
- **Limit**: 5 notifications per event type per window
- **Behavior**: When limit is exceeded, notifications are silently skipped (no error returned)

Rate limiting is applied per event key:
- Help requests: `help_request:{sessionID}`
- Critical errors: `critical_error:{errorType}`
- System alerts: `system_alert:{subject}`

## Email Logging

When MongoDB is provided during initialization, all sent emails are automatically logged to the database by gomail. This includes:

- Timestamp
- From/To addresses
- Subject
- Event type
- List ID

## Error Handling

- **Email failures**: Logged and returned as errors
- **SMS failures**: Logged but don't stop processing (continues to next phone number)
- **Rate limiting**: Silently skips notifications (no error returned)
- **Missing configuration**: SMS is optional; if not configured, SMS notifications are skipped

## Testing

The package includes comprehensive unit tests:

```bash
go test ./internal/notification/... -v
```

Tests cover:
- Service initialization
- Rate limiting behavior
- Email/SMS sending
- Configuration parsing
- Input validation

## Dependencies

- `github.com/real-rm/gomail` - Email service
- `github.com/real-rm/gosms` - SMS service
- `github.com/real-rm/golog` - Logging
- `github.com/real-rm/goconfig` - Configuration
- `github.com/real-rm/gomongo` - MongoDB (optional, for email logging)
