package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/real-rm/chatbox/internal/constants"
	"github.com/real-rm/chatbox/internal/metrics"
	"github.com/real-rm/gohelper"
	"github.com/real-rm/golog"
)

// DifyProvider implements the LLMProvider interface for Dify API
type DifyProvider struct {
	apiKey       string
	endpoint     string
	model        string
	logger       *golog.Logger
	client       *http.Client // used for non-streaming requests (60s timeout)
	streamClient *http.Client // used for streaming requests; ResponseHeaderTimeout guards against hung connections
}

// NewDifyProvider creates a new Dify provider
func NewDifyProvider(apiKey, endpoint, model string, logger *golog.Logger) *DifyProvider {
	return &DifyProvider{
		apiKey:   apiKey,
		endpoint: endpoint,
		model:    model,
		logger:   logger,
		client: &http.Client{
			Timeout: constants.LLMClientTimeout,
		},
		streamClient: &http.Client{
			Timeout:   0, // no deadline; the caller's context controls stream length
			Transport: newStreamTransport(),
		},
	}
}

// difyRequest represents the request format for Dify API
type difyRequest struct {
	Inputs         map[string]string `json:"inputs"`
	Query          string            `json:"query"`
	ResponseMode   string            `json:"response_mode"` // "blocking" or "streaming"
	ConversationID string            `json:"conversation_id,omitempty"`
	User           string            `json:"user"`
}

// difyResponse represents the response format from Dify API
type difyResponse struct {
	Event          string       `json:"event,omitempty"`
	MessageID      string       `json:"message_id,omitempty"`
	ConversationID string       `json:"conversation_id,omitempty"`
	Mode           string       `json:"mode,omitempty"`
	Answer         string       `json:"answer,omitempty"`
	Metadata       difyMetadata `json:"metadata,omitempty"`
	CreatedAt      int64        `json:"created_at,omitempty"`
}

type difyMetadata struct {
	Usage difyUsage `json:"usage,omitempty"`
}

type difyUsage struct {
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
	PromptPrice      string `json:"prompt_price,omitempty"`
	CompletionPrice  string `json:"completion_price,omitempty"`
	TotalPrice       string `json:"total_price,omitempty"`
}

// SendMessage sends a message to Dify and returns the complete response
func (p *DifyProvider) SendMessage(ctx context.Context, req *LLMRequest) (*LLMResponse, error) {
	startTime := time.Now()

	// Dify expects a single query string, so we'll concatenate the messages
	query := p.formatMessages(req.Messages)

	// Create request body
	reqBody := difyRequest{
		Inputs:       make(map[string]string),
		Query:        query,
		ResponseMode: "blocking",
		User:         "user-" + fmt.Sprintf("%d", gohelper.TimeToDateInt(time.Now())),
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.endpoint+"/chat-messages", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	// Send request
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, constants.MaxLLMErrorBodySize))
		return nil, fmt.Errorf("Dify API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse response
	var difyResp difyResponse
	if err := json.NewDecoder(resp.Body).Decode(&difyResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if difyResp.Answer == "" {
		return nil, fmt.Errorf("no answer in response")
	}

	duration := time.Since(startTime)

	return &LLMResponse{
		Content:    difyResp.Answer,
		TokensUsed: difyResp.Metadata.Usage.TotalTokens,
		Duration:   duration,
	}, nil
}

// StreamMessage sends a message to Dify and returns a channel for streaming response chunks
func (p *DifyProvider) StreamMessage(ctx context.Context, req *LLMRequest) (<-chan *LLMChunk, error) {
	// Dify expects a single query string
	query := p.formatMessages(req.Messages)

	// Create request body
	reqBody := difyRequest{
		Inputs:       make(map[string]string),
		Query:        query,
		ResponseMode: "streaming",
		User:         "user-" + fmt.Sprintf("%d", gohelper.TimeToDateInt(time.Now())),
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.endpoint+"/chat-messages", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Accept", "text/event-stream")

	// Send request using streamClient (no transport-level timeout; context cancellation controls the stream)
	resp, err := p.streamClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, constants.MaxLLMErrorBodySize))
		resp.Body.Close()
		return nil, fmt.Errorf("Dify API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Create channel for streaming chunks
	chunkChan := make(chan *LLMChunk)

	go func() {
		defer close(chunkChan)
		defer recoverStreamPanic(chunkChan, "dify", p.logger)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // 1MB max to handle large SSE events
		for scanner.Scan() {
			line := scanner.Text()

			// Skip empty lines
			if line == "" {
				continue
			}

			// Dify streams use "data: " prefix
			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")

			// Parse event
			var event difyResponse
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				// Skip malformed events
				continue
			}

			// Handle different event types
			switch event.Event {
			case "message":
				// Streaming chunk with answer content
				if event.Answer != "" {
					select {
					case chunkChan <- &LLMChunk{Content: event.Answer, Done: false}:
					case <-ctx.Done():
						return
					}
				}
			case "message_end":
				// End of stream
				select {
				case chunkChan <- &LLMChunk{Content: "", Done: true}:
				case <-ctx.Done():
				}
				return
			case "error":
				// Error event
				select {
				case chunkChan <- &LLMChunk{Content: "", Done: true}:
				case <-ctx.Done():
				}
				return
			}
		}

		// Check for scanner errors (e.g. truncated stream from network failure)
		if scanErr := scanner.Err(); scanErr != nil {
			metrics.LLMErrors.WithLabelValues("dify").Inc()
		}

		// Send final chunk if not already sent
		select {
		case chunkChan <- &LLMChunk{Content: "", Done: true}:
		case <-ctx.Done():
		}
	}()

	return chunkChan, nil
}

// GetTokenCount estimates the token count for the given text
// This is a simple approximation: ~4 characters per token for English text
func (p *DifyProvider) GetTokenCount(text string) int {
	// Simple approximation: 1 token ≈ 4 characters
	return len(text) / 4
}

// formatMessages converts ChatMessage array to a single query string
func (p *DifyProvider) formatMessages(messages []ChatMessage) string {
	var parts []string
	for _, msg := range messages {
		// Format: "Role: Content"
		parts = append(parts, fmt.Sprintf("%s: %s", msg.Role, msg.Content))
	}
	return strings.Join(parts, "\n")
}
