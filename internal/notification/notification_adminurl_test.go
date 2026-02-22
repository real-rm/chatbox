package notification

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildHelpRequestHTML_WithAdminURL(t *testing.T) {
	html := buildHelpRequestHTML("user-123", "session-abc", "https://admin.example.com/sessions")

	assert.Contains(t, html, "user-123")
	assert.Contains(t, html, "session-abc")
	assert.Contains(t, html, `href="https://admin.example.com/sessions/session-abc"`)
	assert.Contains(t, html, "View Session")
}

func TestBuildHelpRequestHTML_EmptyAdminURL(t *testing.T) {
	html := buildHelpRequestHTML("user-123", "session-abc", "")

	assert.Contains(t, html, "user-123")
	assert.Contains(t, html, "session-abc")
	// Should NOT contain a hardcoded example URL
	assert.False(t, strings.Contains(html, "admin.example.com"),
		"empty admin URL should not produce a hardcoded link")
	// Should contain fallback text
	assert.Contains(t, html, "Please check the admin panel")
}

func TestBuildHelpRequestHTML_ContainsTimestamp(t *testing.T) {
	html := buildHelpRequestHTML("user-1", "sess-1", "https://admin.test.com")

	// Should contain a timestamp in some form (RFC3339 contains 'T')
	assert.Contains(t, html, "Time:")
}
