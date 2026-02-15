package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/yourusername/chat-websocket/internal/config"
)

var (
	// ErrProviderNotFound is returned when a provider with the given ID is not found
	ErrProviderNotFound = errors.New("LLM provider not found")
	// ErrInvalidModelID is returned when a model ID is empty or invalid
	ErrInvalidModelID = errors.New("invalid model ID")
	// ErrNoProviders is returned when no providers are configured
	ErrNoProviders = errors.New("no LLM providers configured")
)

// LLMProvider defines the interface that all LLM providers must implement
type LLMProvider interface {
	// SendMessage sends a message to the LLM and returns the complete response
	SendMessage(ctx context.Context, req *LLMRequest) (*LLMResponse, error)
	
	// StreamMessage sends a message to the LLM and returns a channel for streaming response chunks
	StreamMessage(ctx context.Context, req *LLMRequest) (<-chan *LLMChunk, error)
	
	// GetTokenCount estimates the token count for the given text
	GetTokenCount(text string) int
}

// LLMRequest represents a request to an LLM provider
type LLMRequest struct {
	ModelID  string        // The model identifier
	Messages []ChatMessage // The conversation history
	Stream   bool          // Whether to stream the response
}

// ChatMessage represents a single message in the conversation
type ChatMessage struct {
	Role    string // "user", "assistant", "system"
	Content string // The message content
}

// LLMResponse represents a complete response from an LLM provider
type LLMResponse struct {
	Content    string        // The generated response text
	TokensUsed int           // Number of tokens consumed
	Duration   time.Duration // Time taken to generate the response
}

// LLMChunk represents a chunk of a streaming response
type LLMChunk struct {
	Content string // The chunk content
	Done    bool   // Whether this is the final chunk
}

// ModelInfo contains information about an available LLM model
type ModelInfo struct {
	ID       string // Unique identifier
	Name     string // Display name
	Type     string // Provider type (openai, anthropic, dify)
	Endpoint string // API endpoint
}

// LLMService manages multiple LLM providers and routes requests to them
type LLMService struct {
	providers map[string]LLMProvider // Map of provider ID to provider instance
	models    map[string]ModelInfo   // Map of model ID to model info
	config    *config.LLMConfig      // LLM configuration
	mu        sync.RWMutex           // Protects concurrent access
}

// NewLLMService creates a new LLM service with the given configuration
func NewLLMService(cfg *config.LLMConfig) (*LLMService, error) {
	if cfg == nil {
		return nil, errors.New("LLM config is required")
	}
	
	if len(cfg.Providers) == 0 {
		return nil, ErrNoProviders
	}
	
	service := &LLMService{
		providers: make(map[string]LLMProvider),
		models:    make(map[string]ModelInfo),
		config:    cfg,
	}
	
	// Register all configured providers
	for _, providerCfg := range cfg.Providers {
		modelInfo := ModelInfo{
			ID:       providerCfg.ID,
			Name:     providerCfg.Name,
			Type:     providerCfg.Type,
			Endpoint: providerCfg.Endpoint,
		}
		service.models[providerCfg.ID] = modelInfo
		
		// Create provider instance based on type
		provider, err := createProvider(providerCfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create provider %s: %w", providerCfg.ID, err)
		}
		
		service.providers[providerCfg.ID] = provider
	}
	
	return service, nil
}

// createProvider creates a provider instance based on the configuration
func createProvider(cfg config.LLMProviderConfig) (LLMProvider, error) {
	switch cfg.Type {
	case "openai":
		return NewOpenAIProvider(cfg.APIKey, cfg.Endpoint, cfg.Model), nil
	case "anthropic":
		return NewAnthropicProvider(cfg.APIKey, cfg.Endpoint, cfg.Model), nil
	case "dify":
		return NewDifyProvider(cfg.APIKey, cfg.Endpoint, cfg.Model), nil
	default:
		return nil, fmt.Errorf("unsupported provider type: %s", cfg.Type)
	}
}

// RegisterProvider registers a provider instance with the service
func (s *LLMService) RegisterProvider(modelID string, provider LLMProvider) error {
	if modelID == "" {
		return ErrInvalidModelID
	}
	if provider == nil {
		return errors.New("provider cannot be nil")
	}
	
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Check if model exists in configuration
	if _, exists := s.models[modelID]; !exists {
		return fmt.Errorf("model %s not found in configuration", modelID)
	}
	
	s.providers[modelID] = provider
	return nil
}

// SendMessage sends a message to the specified LLM model with retry logic and response time tracking
func (s *LLMService) SendMessage(ctx context.Context, modelID string, messages []ChatMessage) (*LLMResponse, error) {
	if modelID == "" {
		return nil, ErrInvalidModelID
	}
	
	provider, err := s.getProvider(modelID)
	if err != nil {
		return nil, err
	}
	
	req := &LLMRequest{
		ModelID:  modelID,
		Messages: messages,
		Stream:   false,
	}
	
	// Implement retry logic with exponential backoff
	var lastErr error
	maxRetries := 3
	baseDelay := 1 * time.Second
	
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Calculate exponential backoff delay
			delay := baseDelay * time.Duration(1<<uint(attempt-1))
			if delay > 30*time.Second {
				delay = 30 * time.Second // Cap at 30 seconds
			}
			
			// Wait before retry
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}
		
		// Measure response time
		startTime := time.Now()
		resp, err := provider.SendMessage(ctx, req)
		duration := time.Since(startTime)
		
		if err == nil {
			// Success - ensure duration is set
			if resp.Duration == 0 {
				resp.Duration = duration
			}
			return resp, nil
		}
		
		lastErr = err
		
		// Check if error is retryable
		if !isRetryableError(err) {
			return nil, fmt.Errorf("non-retryable error: %w", err)
		}
	}
	
	return nil, fmt.Errorf("failed after %d attempts: %w", maxRetries, lastErr)
}

// StreamMessage sends a message to the specified LLM model and returns a streaming channel with retry logic
func (s *LLMService) StreamMessage(ctx context.Context, modelID string, messages []ChatMessage) (<-chan *LLMChunk, error) {
	if modelID == "" {
		return nil, ErrInvalidModelID
	}
	
	provider, err := s.getProvider(modelID)
	if err != nil {
		return nil, err
	}
	
	req := &LLMRequest{
		ModelID:  modelID,
		Messages: messages,
		Stream:   true,
	}
	
	// Implement retry logic with exponential backoff for stream establishment
	var lastErr error
	maxRetries := 3
	baseDelay := 1 * time.Second
	
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Calculate exponential backoff delay
			delay := baseDelay * time.Duration(1<<uint(attempt-1))
			if delay > 30*time.Second {
				delay = 30 * time.Second // Cap at 30 seconds
			}
			
			// Wait before retry
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}
		
		chunkChan, err := provider.StreamMessage(ctx, req)
		if err == nil {
			// Success - wrap channel to track response time
			wrappedChan := make(chan *LLMChunk)
			go func() {
				defer close(wrappedChan)
				for chunk := range chunkChan {
					wrappedChan <- chunk
				}
			}()
			return wrappedChan, nil
		}
		
		lastErr = err
		
		// Check if error is retryable
		if !isRetryableError(err) {
			return nil, fmt.Errorf("non-retryable error: %w", err)
		}
	}
	
	return nil, fmt.Errorf("failed to establish stream after %d attempts: %w", maxRetries, lastErr)
}

// GetAvailableModels returns a list of all configured models
func (s *LLMService) GetAvailableModels() []ModelInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	models := make([]ModelInfo, 0, len(s.models))
	for _, model := range s.models {
		models = append(models, model)
	}
	return models
}

// ValidateModel checks if a model ID exists in the configuration
func (s *LLMService) ValidateModel(modelID string) error {
	if modelID == "" {
		return ErrInvalidModelID
	}
	
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	if _, exists := s.models[modelID]; !exists {
		return fmt.Errorf("%w: %s", ErrProviderNotFound, modelID)
	}
	
	return nil
}

// GetTokenCount estimates the token count for the given text using the specified model
func (s *LLMService) GetTokenCount(modelID string, text string) (int, error) {
	if modelID == "" {
		return 0, ErrInvalidModelID
	}
	
	provider, err := s.getProvider(modelID)
	if err != nil {
		return 0, err
	}
	
	return provider.GetTokenCount(text), nil
}

// getProvider retrieves a provider by model ID
func (s *LLMService) getProvider(modelID string) (LLMProvider, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	provider, exists := s.providers[modelID]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrProviderNotFound, modelID)
	}
	
	return provider, nil
}

// isRetryableError determines if an error should trigger a retry
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	
	errStr := err.Error()
	
	// Network errors are retryable
	if strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "temporary failure") ||
		strings.Contains(errStr, "EOF") {
		return true
	}
	
	// HTTP 5xx errors are retryable
	if strings.Contains(errStr, "status 5") {
		return true
	}
	
	// Rate limit errors (429) are retryable
	if strings.Contains(errStr, "status 429") ||
		strings.Contains(errStr, "rate limit") {
		return true
	}
	
	// Service unavailable errors are retryable
	if strings.Contains(errStr, "unavailable") ||
		strings.Contains(errStr, "overloaded") {
		return true
	}
	
	// Default to non-retryable for unknown errors
	return false
}
