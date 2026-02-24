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
	"github.com/real-rm/golog"
)

// AnthropicProvider implements the LLMProvider interface for Anthropic API
type AnthropicProvider struct {
	apiKey       string
	endpoint     string
	model        string
	logger       *golog.Logger
	client       *http.Client // used for non-streaming requests (60s timeout)
	streamClient *http.Client // used for streaming requests; ResponseHeaderTimeout guards against hung connections
}

// NewAnthropicProvider creates a new Anthropic provider
func NewAnthropicProvider(apiKey, endpoint, model string, logger *golog.Logger) *AnthropicProvider {
	return &AnthropicProvider{
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

// anthropicRequest represents the request format for Anthropic API
type anthropicRequest struct {
	Model     string             `json:"model"`
	Messages  []anthropicMessage `json:"messages"`
	MaxTokens int                `json:"max_tokens"`
	Stream    bool               `json:"stream"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// anthropicResponse represents the response format from Anthropic API
type anthropicResponse struct {
	ID      string             `json:"id"`
	Type    string             `json:"type"`
	Role    string             `json:"role"`
	Content []anthropicContent `json:"content"`
	Model   string             `json:"model"`
	Usage   anthropicUsage     `json:"usage"`
}

type anthropicContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// anthropicStreamEvent represents a streaming event from Anthropic API
type anthropicStreamEvent struct {
	Type         string            `json:"type"`
	Message      anthropicResponse `json:"message,omitempty"`
	Index        int               `json:"index,omitempty"`
	ContentBlock anthropicContent  `json:"content_block,omitempty"`
	Delta        anthropicDelta    `json:"delta,omitempty"`
	Usage        anthropicUsage    `json:"usage,omitempty"`
}

type anthropicDelta struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// SendMessage sends a message to Anthropic and returns the complete response
func (p *AnthropicProvider) SendMessage(ctx context.Context, req *LLMRequest) (*LLMResponse, error) {
	startTime := time.Now()

	// Convert messages to Anthropic format
	messages := make([]anthropicMessage, len(req.Messages))
	for i, msg := range req.Messages {
		// Anthropic uses "user" and "assistant" roles
		role := msg.Role
		if role == "system" {
			// System messages need special handling in Anthropic
			// For simplicity, we'll convert to user message
			role = "user"
		}
		messages[i] = anthropicMessage{
			Role:    role,
			Content: msg.Content,
		}
	}

	// Create request body
	reqBody := anthropicRequest{
		Model:     p.model,
		Messages:  messages,
		MaxTokens: constants.DefaultAnthropicMaxTokens,
		Stream:    false,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.endpoint+"/messages", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	// Send request
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, constants.MaxLLMErrorBodySize))
		return nil, fmt.Errorf("Anthropic API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse response
	var anthropicResp anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&anthropicResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(anthropicResp.Content) == 0 {
		return nil, fmt.Errorf("no content in response")
	}

	duration := time.Since(startTime)
	totalTokens := anthropicResp.Usage.InputTokens + anthropicResp.Usage.OutputTokens

	return &LLMResponse{
		Content:    anthropicResp.Content[0].Text,
		TokensUsed: totalTokens,
		Duration:   duration,
	}, nil
}

// StreamMessage sends a message to Anthropic and returns a channel for streaming response chunks
func (p *AnthropicProvider) StreamMessage(ctx context.Context, req *LLMRequest) (<-chan *LLMChunk, error) {
	// Convert messages to Anthropic format
	messages := make([]anthropicMessage, len(req.Messages))
	for i, msg := range req.Messages {
		role := msg.Role
		if role == "system" {
			role = "user"
		}
		messages[i] = anthropicMessage{
			Role:    role,
			Content: msg.Content,
		}
	}

	// Create request body
	reqBody := anthropicRequest{
		Model:     p.model,
		Messages:  messages,
		MaxTokens: constants.DefaultAnthropicMaxTokens,
		Stream:    true,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.endpoint+"/messages", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("Accept", "text/event-stream")

	// Send request using streamClient (no transport-level timeout; context cancellation controls the stream)
	resp, err := p.streamClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, constants.MaxLLMErrorBodySize))
		resp.Body.Close()
		return nil, fmt.Errorf("Anthropic API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Create channel for streaming chunks
	chunkChan := make(chan *LLMChunk)

	go func() {
		defer close(chunkChan)
		defer recoverStreamPanic(chunkChan, "anthropic", p.logger)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // 1MB max to handle large SSE events
		for scanner.Scan() {
			line := scanner.Text()

			// Skip empty lines
			if line == "" {
				continue
			}

			// Anthropic streams use "event: " and "data: " format
			if strings.HasPrefix(line, "event: ") {
				// Event type line, skip for now
				continue
			}

			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")

			// Parse event
			var event anthropicStreamEvent
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				// Skip malformed events
				continue
			}

			// Handle different event types
			switch event.Type {
			case "content_block_delta":
				if event.Delta.Text != "" {
					select {
					case chunkChan <- &LLMChunk{Content: event.Delta.Text, Done: false}:
					case <-ctx.Done():
						return
					}
				}
			case "message_stop":
				select {
				case chunkChan <- &LLMChunk{Content: "", Done: true}:
				case <-ctx.Done():
				}
				return
			}
		}

		// Check for scanner errors (e.g. truncated stream from network failure)
		if scanErr := scanner.Err(); scanErr != nil {
			metrics.LLMErrors.WithLabelValues("anthropic").Inc()
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
func (p *AnthropicProvider) GetTokenCount(text string) int {
	// Simple approximation: 1 token ≈ 4 characters
	return len(text) / 4
}
