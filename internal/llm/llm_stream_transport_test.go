package llm

import (
	"net/http"
	"testing"
	"time"

	"github.com/real-rm/chatbox/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStreamClient_ResponseHeaderTimeout verifies that all three LLM providers
// configure their streamClient with a ResponseHeaderTimeout on the transport.
// This protects against hung connections where the server accepts the TCP
// connection but never sends the first response byte.
func TestStreamClient_ResponseHeaderTimeout(t *testing.T) {
	logger := createTestLogger()

	providers := []struct {
		name   string
		client *http.Client
	}{
		{
			name:   "openai",
			client: NewOpenAIProvider("key", "gpt-4", "https://api.openai.com", logger).streamClient,
		},
		{
			name:   "anthropic",
			client: NewAnthropicProvider("key", "claude-3", "https://api.anthropic.com", logger).streamClient,
		},
		{
			name:   "dify",
			client: NewDifyProvider("key", "https://api.dify.ai", "model", logger).streamClient,
		},
	}

	for _, p := range providers {
		t.Run(p.name, func(t *testing.T) {
			require.NotNil(t, p.client, "streamClient must not be nil")

			transport, ok := p.client.Transport.(*http.Transport)
			require.True(t, ok, "streamClient.Transport must be *http.Transport")
			assert.Equal(t, constants.LLMStreamHeaderTimeout, transport.ResponseHeaderTimeout,
				"ResponseHeaderTimeout must equal constants.LLMStreamHeaderTimeout")
			assert.Greater(t, transport.ResponseHeaderTimeout, time.Duration(0),
				"ResponseHeaderTimeout must be positive")
		})
	}
}
