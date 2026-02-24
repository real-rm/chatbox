package llm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/real-rm/chatbox/internal/constants"
	"github.com/real-rm/chatbox/internal/metrics"
	"github.com/real-rm/goconfig"
	"github.com/real-rm/golog"
)

// newStreamTransport returns an HTTP transport cloned from http.DefaultTransport
// with ResponseHeaderTimeout set to protect against hung streaming connections
// (server accepts the TCP connection but never sends the first response byte).
func newStreamTransport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.ResponseHeaderTimeout = constants.LLMStreamHeaderTimeout
	return t
}

var (
	// ErrProviderNotFound is returned when a provider with the given ID is not found
	ErrProviderNotFound = errors.New("LLM provider not found")
	// ErrInvalidModelID is returned when a model ID is empty or invalid
	ErrInvalidModelID = errors.New("invalid model ID")
	// ErrNoProviders is returned when no providers are configured
	ErrNoProviders = errors.New("no LLM providers configured")
)

// LLMProviderConfig holds configuration for a single LLM provider
type LLMProviderConfig struct {
	ID       string
	Name     string
	Type     string // "openai", "anthropic", "dify"
	Endpoint string
	APIKey   string
	Model    string
}

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
	providers map[string]LLMProvider   // Map of provider ID to provider instance
	models    map[string]ModelInfo     // Map of model ID to model info
	config    *goconfig.ConfigAccessor // Configuration accessor
	logger    *golog.Logger            // Logger for LLM operations
	mu        sync.RWMutex             // Protects concurrent access
}

// NewLLMService creates a new LLM service with the given configuration accessor
func NewLLMService(cfg *goconfig.ConfigAccessor, logger *golog.Logger) (*LLMService, error) {
	if cfg == nil {
		return nil, errors.New("config accessor is required")
	}
	if logger == nil {
		return nil, errors.New("logger is required")
	}

	llmLogger := logger.WithGroup("llm")

	// Load LLM providers from config
	providers, err := loadLLMProviders(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to load LLM providers: %w", err)
	}

	if len(providers) == 0 {
		return nil, ErrNoProviders
	}

	service := &LLMService{
		providers: make(map[string]LLMProvider),
		models:    make(map[string]ModelInfo),
		config:    cfg,
		logger:    llmLogger,
	}

	// Register all configured providers
	for _, providerCfg := range providers {
		modelInfo := ModelInfo{
			ID:       providerCfg.ID,
			Name:     providerCfg.Name,
			Type:     providerCfg.Type,
			Endpoint: providerCfg.Endpoint,
		}
		service.models[providerCfg.ID] = modelInfo

		// Create provider instance based on type
		provider, err := createProvider(providerCfg, llmLogger)
		if err != nil {
			return nil, fmt.Errorf("failed to create provider %s: %w", providerCfg.ID, err)
		}

		service.providers[providerCfg.ID] = provider
		llmLogger.Info("Registered LLM provider", "provider_id", providerCfg.ID, "type", providerCfg.Type)
	}

	return service, nil
}

// loadLLMProviders loads LLM provider configurations from ConfigAccessor
// Priority: Environment variables > Config file
// This allows Kubernetes secrets to override config.toml values
func loadLLMProviders(cfg *goconfig.ConfigAccessor) ([]LLMProviderConfig, error) {
	// Get the llm.providers array from config
	providersConfig, err := cfg.Config("llm.providers")
	if err != nil {
		// If llm.providers doesn't exist, return empty slice
		return []LLMProviderConfig{}, nil
	}

	// Handle nil case
	if providersConfig == nil {
		return []LLMProviderConfig{}, nil
	}

	// Convert to slice of provider configs
	providersSlice, ok := providersConfig.([]interface{})
	if !ok {
		return nil, errors.New("llm.providers is not an array")
	}

	providers := make([]LLMProviderConfig, 0, len(providersSlice))
	for i, p := range providersSlice {
		providerMap, ok := p.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("provider %d is not a map", i)
		}

		provider := LLMProviderConfig{
			ID:       getStringFromMap(providerMap, "id"),
			Name:     getStringFromMap(providerMap, "name"),
			Type:     getStringFromMap(providerMap, "type"),
			Endpoint: getStringFromMap(providerMap, "endpoint"),
			APIKey:   getStringFromMap(providerMap, "apiKey"),
			Model:    getStringFromMap(providerMap, "model"),
		}

		// Override API key from environment variable if available
		// Format: LLM_PROVIDER_<INDEX>_API_KEY (e.g., LLM_PROVIDER_1_API_KEY)
		envKey := fmt.Sprintf("LLM_PROVIDER_%d_API_KEY", i+1)
		if envAPIKey := os.Getenv(envKey); envAPIKey != "" {
			provider.APIKey = envAPIKey
		}

		// Validate required fields
		if provider.ID == "" {
			return nil, fmt.Errorf("provider %d: ID is required", i)
		}
		if provider.Name == "" {
			return nil, fmt.Errorf("provider %d: name is required", i)
		}
		if provider.Type == "" {
			return nil, fmt.Errorf("provider %d: type is required", i)
		}
		if provider.Endpoint == "" {
			return nil, fmt.Errorf("provider %d: endpoint is required", i)
		}
		if err := ValidateEndpoint(provider.Endpoint); err != nil {
			return nil, fmt.Errorf("provider %d: %w", i, err)
		}
		if provider.APIKey == "" {
			return nil, fmt.Errorf("provider %d: API key is required", i)
		}

		providers = append(providers, provider)
	}

	return providers, nil
}

// getStringFromMap safely extracts a string value from a map
func getStringFromMap(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// createProvider creates a provider instance based on the configuration.
// The logger is passed to the provider so it can log panic stack traces via recoverStreamPanic.
func createProvider(cfg LLMProviderConfig, logger *golog.Logger) (LLMProvider, error) {
	switch cfg.Type {
	case "openai":
		return NewOpenAIProvider(cfg.APIKey, cfg.Endpoint, cfg.Model, logger), nil
	case "anthropic":
		return NewAnthropicProvider(cfg.APIKey, cfg.Endpoint, cfg.Model, logger), nil
	case "dify":
		return NewDifyProvider(cfg.APIKey, cfg.Endpoint, cfg.Model, logger), nil
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

// registerProviderUnsafe registers a provider without checking if the model exists in configuration.
// This is intended for testing purposes only.
func (s *LLMService) registerProviderUnsafe(modelID string, provider LLMProvider) error {
	if modelID == "" {
		return ErrInvalidModelID
	}
	if provider == nil {
		return errors.New("provider cannot be nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Add model info if it doesn't exist (for testing)
	if _, exists := s.models[modelID]; !exists {
		s.models[modelID] = ModelInfo{
			ID:   modelID,
			Name: modelID,
			Type: "test",
		}
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

	// Get provider name for metrics
	providerName := s.getProviderName(modelID)

	req := &LLMRequest{
		ModelID:  modelID,
		Messages: messages,
		Stream:   false,
	}

	// Implement retry logic with exponential backoff
	var lastErr error
	maxRetries := constants.MaxRetryAttempts
	baseDelay := constants.LLMInitialRetryDelay

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Calculate exponential backoff delay
			delay := baseDelay * time.Duration(1<<uint(attempt-1))
			if delay > constants.LLMMaxRetryDelay {
				delay = constants.LLMMaxRetryDelay
			}

			s.logger.Info("Retrying LLM request", "model_id", modelID, "attempt", attempt+1, "delay", delay)

			// Wait before retry
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		// Increment LLM requests metric
		metrics.LLMRequests.WithLabelValues(providerName).Inc()

		// Measure response time
		startTime := time.Now()
		resp, err := provider.SendMessage(ctx, req)
		duration := time.Since(startTime)

		// Record latency metric
		metrics.LLMLatency.WithLabelValues(providerName).Observe(duration.Seconds())

		if err == nil {
			// Success - ensure duration is set
			if resp.Duration == 0 {
				resp.Duration = duration
			}

			// Record token usage metric
			if resp.TokensUsed > 0 {
				metrics.TokensUsed.WithLabelValues(providerName).Add(float64(resp.TokensUsed))
			}

			s.logger.Info("LLM request successful", "model_id", modelID, "duration", duration, "tokens", resp.TokensUsed)
			return resp, nil
		}

		lastErr = err

		// Increment LLM errors metric
		metrics.LLMErrors.WithLabelValues(providerName).Inc()

		s.logger.Warn("LLM request failed", "model_id", modelID, "attempt", attempt+1, "error", err)

		// Check if error is retryable
		if !isRetryableError(err) {
			return nil, fmt.Errorf("non-retryable error: %w", err)
		}
	}

	s.logger.Error("LLM request failed after all retries", "model_id", modelID, "max_retries", maxRetries, "error", lastErr)
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

	// Get provider name for metrics
	providerName := s.getProviderName(modelID)

	req := &LLMRequest{
		ModelID:  modelID,
		Messages: messages,
		Stream:   true,
	}

	// Implement retry logic with exponential backoff for stream establishment
	var lastErr error
	maxRetries := constants.MaxRetryAttempts
	baseDelay := constants.LLMInitialRetryDelay

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Calculate exponential backoff delay
			delay := baseDelay * time.Duration(1<<uint(attempt-1))
			if delay > constants.LLMMaxRetryDelay {
				delay = constants.LLMMaxRetryDelay
			}

			s.logger.Info("Retrying LLM stream request", "model_id", modelID, "attempt", attempt+1, "delay", delay)

			// Wait before retry
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		// Increment LLM requests metric
		metrics.LLMRequests.WithLabelValues(providerName).Inc()

		// Track start time for latency measurement
		startTime := time.Now()

		chunkChan, err := provider.StreamMessage(ctx, req)
		if err == nil {
			// Success - wrap channel to track response time
			s.logger.Info("LLM stream established", "model_id", modelID)
			wrappedChan := make(chan *LLMChunk)
			go func() {
				defer close(wrappedChan)
				defer recoverStreamPanic(wrappedChan, providerName, s.logger)
				firstChunk := true
				for chunk := range chunkChan {
					// Record latency for first chunk (time to first token)
					if firstChunk {
						duration := time.Since(startTime)
						metrics.LLMLatency.WithLabelValues(providerName).Observe(duration.Seconds())
						firstChunk = false
					}
					select {
					case wrappedChan <- chunk:
					case <-ctx.Done():
						return
					}
				}
			}()
			return wrappedChan, nil
		}

		lastErr = err

		// Increment LLM errors metric
		metrics.LLMErrors.WithLabelValues(providerName).Inc()

		s.logger.Warn("LLM stream request failed", "model_id", modelID, "attempt", attempt+1, "error", err)

		// Check if error is retryable
		if !isRetryableError(err) {
			return nil, fmt.Errorf("non-retryable error: %w", err)
		}
	}

	s.logger.Error("LLM stream failed after all retries", "model_id", modelID, "max_retries", maxRetries, "error", lastErr)
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

// getProviderName returns the provider name (type) for a given model ID
// Returns "unknown" if the model is not found
func (s *LLMService) getProviderName(modelID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Check the models map to find the matching model
	if modelInfo, exists := s.models[modelID]; exists {
		return modelInfo.Type
	}

	return "unknown"
}

// ValidateEndpoint validates that an LLM provider endpoint URL uses HTTPS and has a host.
// This prevents SSRF by rejecting non-HTTPS endpoints at startup.
func ValidateEndpoint(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("invalid endpoint URL: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("endpoint must use https scheme, got %q", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("endpoint must have a host")
	}
	return nil
}

// recoverStreamPanic handles panic recovery in streaming goroutines.
// On panic it logs the panic value and stack trace, increments the error metric,
// and sends a Done chunk to unblock downstream consumers.
func recoverStreamPanic(chunkChan chan<- *LLMChunk, component string, logger *golog.Logger) {
	if r := recover(); r != nil {
		logger.Error("Panic recovered in LLM streaming goroutine",
			"component", component,
			"panic", fmt.Sprintf("%v", r),
			"stack", string(debug.Stack()))
		metrics.LLMErrors.WithLabelValues(component).Inc()
		// Best-effort send; if the channel is already closed or full, skip.
		select {
		case chunkChan <- &LLMChunk{Done: true}:
		default:
		}
	}
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
