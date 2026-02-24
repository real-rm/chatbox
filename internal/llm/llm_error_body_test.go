package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/real-rm/chatbox/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAI_ErrorBodyTruncated(t *testing.T) {
	// Create a server that returns a large error body
	largeBody := strings.Repeat("X", 5000) // 5KB >> MaxLLMErrorBodySize (1KB)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(largeBody))
	}))
	defer server.Close()

	provider := NewOpenAIProvider("test-key", server.URL, "gpt-4", createTestLogger())

	_, err := provider.SendMessage(context.Background(), &LLMRequest{
		ModelID:  "gpt-4",
		Messages: []ChatMessage{{Role: "user", Content: "hello"}},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "OpenAI API error")
	// Error body should be truncated to MaxLLMErrorBodySize
	assert.LessOrEqual(t, len(err.Error()), constants.MaxLLMErrorBodySize+200, // +200 for prefix text
		"error body should be truncated")
}

func TestAnthropic_ErrorBodyTruncated(t *testing.T) {
	largeBody := strings.Repeat("Y", 5000)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(largeBody))
	}))
	defer server.Close()

	provider := NewAnthropicProvider("test-key", server.URL, "claude-3", createTestLogger())

	_, err := provider.SendMessage(context.Background(), &LLMRequest{
		ModelID:  "claude-3",
		Messages: []ChatMessage{{Role: "user", Content: "hello"}},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Anthropic API error")
	assert.LessOrEqual(t, len(err.Error()), constants.MaxLLMErrorBodySize+200)
}

func TestDify_ErrorBodyTruncated(t *testing.T) {
	largeBody := strings.Repeat("Z", 5000)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(largeBody))
	}))
	defer server.Close()

	provider := NewDifyProvider("test-key", server.URL, "assistant", createTestLogger())

	_, err := provider.SendMessage(context.Background(), &LLMRequest{
		ModelID:  "assistant",
		Messages: []ChatMessage{{Role: "user", Content: "hello"}},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Dify API error")
	assert.LessOrEqual(t, len(err.Error()), constants.MaxLLMErrorBodySize+200)
}

func TestOpenAI_StreamErrorBodyTruncated(t *testing.T) {
	largeBody := strings.Repeat("A", 5000)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(largeBody))
	}))
	defer server.Close()

	provider := NewOpenAIProvider("test-key", server.URL, "gpt-4", createTestLogger())

	_, err := provider.StreamMessage(context.Background(), &LLMRequest{
		ModelID:  "gpt-4",
		Messages: []ChatMessage{{Role: "user", Content: "hello"}},
		Stream:   true,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "OpenAI API error")
	assert.LessOrEqual(t, len(err.Error()), constants.MaxLLMErrorBodySize+200)
}
