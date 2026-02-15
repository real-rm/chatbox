package message

import (
	"strings"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// Feature: chat-application-websocket, Property 42: Input Sanitization
// **Validates: Requirements 13.2**
//
// For any user input received, the WebSocket_Server should validate and sanitize it
// before processing to prevent injection attacks.
func TestProperty_InputSanitization(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 20
	properties := gopter.NewProperties(parameters)

	properties.Property("sanitize removes malicious content from all fields", prop.ForAll(
		func(content string, sessionID string, fileID string, modelID string) bool {
			// Create message with potentially malicious content
			msg := &Message{
				Type:      TypeUserMessage,
				SessionID: sessionID,
				Content:   content,
				FileID:    fileID,
				ModelID:   modelID,
				Sender:    SenderUser,
			}

			// Sanitize the message
			msg.Sanitize()

			// Verify no script tags remain in any field
			if containsUnsafeHTML(msg.Content) {
				return false
			}
			if containsUnsafeHTML(msg.SessionID) {
				return false
			}
			if containsUnsafeHTML(msg.FileID) {
				return false
			}
			if containsUnsafeHTML(msg.ModelID) {
				return false
			}

			// Verify no null bytes remain
			if strings.Contains(msg.Content, "\x00") {
				return false
			}
			if strings.Contains(msg.SessionID, "\x00") {
				return false
			}

			// Verify leading/trailing whitespace is trimmed
			if msg.Content != strings.TrimSpace(msg.Content) {
				return false
			}
			if msg.SessionID != strings.TrimSpace(msg.SessionID) {
				return false
			}

			return true
		},
		genMaliciousString(),
		genMaliciousString(),
		genMaliciousString(),
		genMaliciousString(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: chat-application-websocket, Property 42: Input Sanitization (XSS Prevention)
// **Validates: Requirements 13.2**
//
// For any user input containing XSS attack vectors, the Sanitize() method should
// properly escape or remove the malicious content.
func TestProperty_XSSPrevention(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 20
	properties := gopter.NewProperties(parameters)

	properties.Property("sanitize prevents XSS attacks", prop.ForAll(
		func(xssVector string) bool {
			msg := &Message{
				Type:    TypeUserMessage,
				Content: xssVector,
				Sender:  SenderUser,
			}

			// Sanitize the message
			msg.Sanitize()

			// Verify dangerous patterns are escaped
			// Script tags should be escaped
			if strings.Contains(msg.Content, "<script>") || strings.Contains(msg.Content, "</script>") {
				return false
			}

			// Event handlers should be escaped
			if strings.Contains(msg.Content, "onerror=") && !strings.Contains(msg.Content, "&") {
				return false
			}
			if strings.Contains(msg.Content, "onclick=") && !strings.Contains(msg.Content, "&") {
				return false
			}

			// JavaScript protocol should be escaped
			if strings.Contains(msg.Content, "javascript:") && strings.Contains(msg.Content, "<") {
				// If there's a tag with javascript:, it should be escaped
				return false
			}

			return true
		},
		genXSSVector(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: chat-application-websocket, Property 42: Input Sanitization (SQL Injection Prevention)
// **Validates: Requirements 13.2**
//
// For any user input containing SQL injection patterns, the Sanitize() method should
// properly escape the malicious content.
func TestProperty_SQLInjectionPrevention(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 20
	properties := gopter.NewProperties(parameters)

	properties.Property("sanitize escapes SQL injection patterns", prop.ForAll(
		func(sqlVector string) bool {
			msg := &Message{
				Type:    TypeUserMessage,
				Content: sqlVector,
				Sender:  SenderUser,
			}

			// Sanitize the message
			msg.Sanitize()

			// Verify single quotes are escaped (HTML entity &#39;)
			if strings.Contains(sqlVector, "'") {
				// After sanitization, single quotes should be HTML escaped
				if strings.Contains(msg.Content, "'") && !strings.Contains(msg.Content, "&#39;") {
					return false
				}
			}

			// Verify double quotes are escaped (HTML entity &#34;)
			if strings.Contains(sqlVector, "\"") {
				if strings.Contains(msg.Content, "\"") && !strings.Contains(msg.Content, "&#34;") {
					return false
				}
			}

			return true
		},
		genSQLInjectionVector(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: chat-application-websocket, Property 42: Input Sanitization (Metadata)
// **Validates: Requirements 13.2**
//
// For any metadata with malicious content, the Sanitize() method should properly
// escape both keys and values.
func TestProperty_MetadataSanitization(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 20
	properties := gopter.NewProperties(parameters)

	properties.Property("sanitize escapes metadata keys and values", prop.ForAll(
		func(key string, value string) bool {
			if key == "" {
				return true // Skip empty keys
			}

			msg := &Message{
				Type:   TypeUserMessage,
				Sender: SenderUser,
				Metadata: map[string]string{
					key: value,
				},
			}

			// Sanitize the message
			msg.Sanitize()

			// Verify metadata is sanitized
			for k, v := range msg.Metadata {
				// Keys and values should not contain unsafe HTML
				if containsUnsafeHTML(k) || containsUnsafeHTML(v) {
					return false
				}

				// Keys and values should not contain null bytes
				if strings.Contains(k, "\x00") || strings.Contains(v, "\x00") {
					return false
				}
			}

			return true
		},
		genMaliciousString(),
		genMaliciousString(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: chat-application-websocket, Property 42: Input Sanitization (Error Info)
// **Validates: Requirements 13.2**
//
// For any error info with malicious content, the Sanitize() method should properly
// escape the error code and message.
func TestProperty_ErrorInfoSanitization(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 20
	properties := gopter.NewProperties(parameters)

	properties.Property("sanitize escapes error info fields", prop.ForAll(
		func(code string, message string) bool {
			if code == "" || message == "" {
				return true // Skip empty values
			}

			msg := &Message{
				Type:   TypeError,
				Sender: SenderUser,
				Error: &ErrorInfo{
					Code:    code,
					Message: message,
				},
			}

			// Sanitize the message
			msg.Sanitize()

			// Verify error info is sanitized
			if msg.Error != nil {
				if containsUnsafeHTML(msg.Error.Code) || containsUnsafeHTML(msg.Error.Message) {
					return false
				}

				if strings.Contains(msg.Error.Code, "\x00") || strings.Contains(msg.Error.Message, "\x00") {
					return false
				}
			}

			return true
		},
		genMaliciousString(),
		genMaliciousString(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// genMaliciousString generates strings with potentially malicious content
func genMaliciousString() gopter.Gen {
	return gen.OneGenOf(
		// XSS vectors
		gen.Const("<script>alert('XSS')</script>"),
		gen.Const("<img src=x onerror=alert(1)>"),
		gen.Const("<a href='javascript:alert(1)'>Click</a>"),
		gen.Const("<iframe src='javascript:alert(1)'>"),
		gen.Const("<body onload=alert('XSS')>"),
		gen.Const("<svg onload=alert(1)>"),
		gen.Const("<input onfocus=alert(1) autofocus>"),

		// SQL injection vectors
		gen.Const("'; DROP TABLE users; --"),
		gen.Const("1' UNION SELECT * FROM users--"),
		gen.Const("admin'--"),
		gen.Const("' OR '1'='1"),
		gen.Const("1; DELETE FROM users WHERE 1=1--"),

		// Special characters
		gen.Const("test\x00data"),
		gen.Const("  leading and trailing spaces  "),
		gen.Const("<>&\"'"),
		gen.Const("test\ndata\twith\rspecial"),

		// Normal strings (should pass through safely)
		gen.AlphaString(),
		gen.Const("Hello, world!"),
		gen.Const("Normal text with numbers 123"),
		gen.Const("Unicode: 世界 🌍"),

		// Empty string
		gen.Const(""),
	)
}

// genXSSVector generates XSS attack vectors
func genXSSVector() gopter.Gen {
	return gen.OneGenOf(
		gen.Const("<script>alert('XSS')</script>"),
		gen.Const("<script>document.cookie</script>"),
		gen.Const("<img src=x onerror=alert(1)>"),
		gen.Const("<img src=x onerror='alert(1)'>"),
		gen.Const("<a href='javascript:alert(1)'>Click</a>"),
		gen.Const("<iframe src='javascript:alert(1)'>"),
		gen.Const("<body onload=alert('XSS')>"),
		gen.Const("<svg onload=alert(1)>"),
		gen.Const("<input onfocus=alert(1) autofocus>"),
		gen.Const("<div onclick=alert(1)>Click</div>"),
		gen.Const("<style>@import'javascript:alert(1)';</style>"),
		gen.Const("<object data='javascript:alert(1)'>"),
		gen.Const("<embed src='javascript:alert(1)'>"),
		gen.Const("<link rel='stylesheet' href='javascript:alert(1)'>"),
		gen.Const("<meta http-equiv='refresh' content='0;url=javascript:alert(1)'>"),
	)
}

// genSQLInjectionVector generates SQL injection attack vectors
func genSQLInjectionVector() gopter.Gen {
	return gen.OneGenOf(
		gen.Const("'; DROP TABLE users; --"),
		gen.Const("1' UNION SELECT * FROM users--"),
		gen.Const("admin'--"),
		gen.Const("' OR '1'='1"),
		gen.Const("1; DELETE FROM users WHERE 1=1--"),
		gen.Const("' OR 1=1--"),
		gen.Const("1' AND '1'='1"),
		gen.Const("'; EXEC sp_MSForEachTable 'DROP TABLE ?'; --"),
		gen.Const("1' UNION ALL SELECT NULL,NULL,NULL--"),
		gen.Const("admin' OR '1'='1'--"),
		gen.Const("1'; WAITFOR DELAY '00:00:05'--"),
		gen.Const("1' AND SLEEP(5)--"),
	)
}

// containsUnsafeHTML checks if a string contains unsafe HTML patterns
func containsUnsafeHTML(s string) bool {
	// Check for unescaped script tags
	if strings.Contains(s, "<script>") || strings.Contains(s, "</script>") {
		return true
	}

	// Check for unescaped iframe tags
	if strings.Contains(s, "<iframe") && !strings.Contains(s, "&lt;iframe") {
		return true
	}

	// Check for unescaped img tags with onerror
	if strings.Contains(s, "<img") && strings.Contains(s, "onerror") && !strings.Contains(s, "&lt;") {
		return true
	}

	// Check for unescaped event handlers in tags
	unsafePatterns := []string{
		"<body onload",
		"<svg onload",
		"<input onfocus",
		"<div onclick",
	}
	for _, pattern := range unsafePatterns {
		if strings.Contains(s, pattern) && !strings.Contains(s, "&lt;") {
			return true
		}
	}

	return false
}
