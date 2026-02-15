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
)

// OpenAIProvider implements the LLMProvider interface for OpenAI API
type OpenAIProvider struct {
	apiKey   string
	endpoint string
	model    string
	client   *http.Client
}

// NewOpenAIProvider creates a new OpenAI provider
func NewOpenAIProvider(apiKey, endpoint, model string) *OpenAIProvider {
	return &OpenAIProvider{
		apiKey:   apiKey,
		endpoint: endpoint,
		model:    model,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// openAIRequest represents the request format for OpenAI API
type openAIRequest struct {
	Model    string              `json:"model"`
	Messages []openAIMessage     `json:"messages"`
	Stream   bool                `json:"stream"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openAIResponse represents the response format from OpenAI API
type openAIResponse struct {
	ID      string           `json:"id"`
	Object  string           `json:"object"`
	Created int64            `json:"created"`
	Model   string           `json:"model"`
	Choices []openAIChoice   `json:"choices"`
	Usage   openAIUsage      `json:"usage"`
}

type openAIChoice struct {
	Index        int           `json:"index"`
	Message      openAIMessage `json:"message"`
	Delta        openAIMessage `json:"delta"`
	FinishReason string        `json:"finish_reason"`
}

type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// SendMessage sends a message to OpenAI and returns the complete response
func (p *OpenAIProvider) SendMessage(ctx context.Context, req *LLMRequest) (*LLMResponse, error) {
	startTime := time.Now()
	
	// Convert messages to OpenAI format
	messages := make([]openAIMessage, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = openAIMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}
	
	// Create request body
	reqBody := openAIRequest{
		Model:    p.model,
		Messages: messages,
		Stream:   false,
	}
	
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	
	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.endpoint+"/chat/completions", bytes.NewReader(bodyBytes))
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
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenAI API error (status %d): %s", resp.StatusCode, string(body))
	}
	
	// Parse response
	var openAIResp openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&openAIResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	
	if len(openAIResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}
	
	duration := time.Since(startTime)
	
	return &LLMResponse{
		Content:    openAIResp.Choices[0].Message.Content,
		TokensUsed: openAIResp.Usage.TotalTokens,
		Duration:   duration,
	}, nil
}

// StreamMessage sends a message to OpenAI and returns a channel for streaming response chunks
func (p *OpenAIProvider) StreamMessage(ctx context.Context, req *LLMRequest) (<-chan *LLMChunk, error) {
	// Convert messages to OpenAI format
	messages := make([]openAIMessage, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = openAIMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}
	
	// Create request body
	reqBody := openAIRequest{
		Model:    p.model,
		Messages: messages,
		Stream:   true,
	}
	
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	
	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.endpoint+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Accept", "text/event-stream")
	
	// Send request
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("OpenAI API error (status %d): %s", resp.StatusCode, string(body))
	}
	
	// Create channel for streaming chunks
	chunkChan := make(chan *LLMChunk)
	
	go func() {
		defer close(chunkChan)
		defer resp.Body.Close()
		
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			
			// Skip empty lines
			if line == "" {
				continue
			}
			
			// OpenAI streams use "data: " prefix
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			
			data := strings.TrimPrefix(line, "data: ")
			
			// Check for stream end
			if data == "[DONE]" {
				chunkChan <- &LLMChunk{Content: "", Done: true}
				return
			}
			
			// Parse chunk
			var chunkResp openAIResponse
			if err := json.Unmarshal([]byte(data), &chunkResp); err != nil {
				// Skip malformed chunks
				continue
			}
			
			if len(chunkResp.Choices) > 0 {
				content := chunkResp.Choices[0].Delta.Content
				if content != "" {
					chunkChan <- &LLMChunk{Content: content, Done: false}
				}
			}
		}
		
		// Send final chunk if not already sent
		chunkChan <- &LLMChunk{Content: "", Done: true}
	}()
	
	return chunkChan, nil
}

// GetTokenCount estimates the token count for the given text
// This is a simple approximation: ~4 characters per token for English text
func (p *OpenAIProvider) GetTokenCount(text string) int {
	// Simple approximation: 1 token ≈ 4 characters
	return len(text) / 4
}
