package llm

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/real-rm/goconfig"
	"github.com/real-rm/golog"
)

// createTestConfig creates a test config accessor with the given providers
func createTestConfig(providers []LLMProviderConfig) *goconfig.ConfigAccessor {
	// Create a temporary TOML file with the provider configuration
	content := ""
	if len(providers) > 0 {
		for i, p := range providers {
			if i == 0 {
				content += "[[llm.providers]]\n"
			} else {
				content += "\n[[llm.providers]]\n"
			}
			content += fmt.Sprintf("id = \"%s\"\n", p.ID)
			content += fmt.Sprintf("name = \"%s\"\n", p.Name)
			content += fmt.Sprintf("type = \"%s\"\n", p.Type)
			content += fmt.Sprintf("endpoint = \"%s\"\n", p.Endpoint)
			content += fmt.Sprintf("apiKey = \"%s\"\n", p.APIKey)
			if p.Model != "" {
				content += fmt.Sprintf("model = \"%s\"\n", p.Model)
			}
		}
	}
	
	// Create temporary file
	tmpfile, err := os.CreateTemp("", "llm-test-config-*.toml")
	if err != nil {
		panic(fmt.Sprintf("Failed to create temp config file: %v", err))
	}
	
	if _, err := tmpfile.Write([]byte(content)); err != nil {
		os.Remove(tmpfile.Name())
		panic(fmt.Sprintf("Failed to write temp config file: %v", err))
	}
	tmpfile.Close()
	
	// Clear any existing config by unsetting the environment variable first
	os.Unsetenv("RMBASE_FILE_CFG")
	os.Unsetenv("RMBASE_FOLDER_CFG")
	
	// Set the config file path
	os.Setenv("RMBASE_FILE_CFG", tmpfile.Name())
	
	// Force reload the config by calling LoadConfig
	// Note: goconfig may cache internally, so we need to ensure fresh load
	if err := goconfig.LoadConfig(); err != nil {
		os.Remove(tmpfile.Name())
		panic(fmt.Sprintf("Failed to load config: %v", err))
	}
	
	cfg, err := goconfig.Default()
	if err != nil {
		os.Remove(tmpfile.Name())
		panic(fmt.Sprintf("Failed to get config accessor: %v", err))
	}
	
	// Keep the temp file around for the duration of the test
	// It will be cleaned up by the OS eventually
	
	return cfg
}

// createTestLogger creates a test logger
func createTestLogger() *golog.Logger {
	// Create a temporary directory for logs
	tmpDir, err := os.MkdirTemp("", "llm-test-logs-*")
	if err != nil {
		panic(fmt.Sprintf("Failed to create temp log dir: %v", err))
	}
	
	logger, err := golog.InitLog(golog.LogConfig{
		Dir:            tmpDir,
		Level:          "error", // Only log errors during tests
		StandardOutput: false,
	})
	if err != nil {
		os.RemoveAll(tmpDir)
		panic(fmt.Sprintf("Failed to create test logger: %v", err))
	}
	
	// Clean up the temp directory after a short delay (logger might still be writing)
	go func() {
		time.Sleep(100 * time.Millisecond)
		os.RemoveAll(tmpDir)
	}()
	
	return logger
}

// MockLLMProvider is a mock implementation of LLMProvider for testing
type MockLLMProvider struct {
	sendMessageFunc    func(ctx context.Context, req *LLMRequest) (*LLMResponse, error)
	streamMessageFunc  func(ctx context.Context, req *LLMRequest) (<-chan *LLMChunk, error)
	getTokenCountFunc  func(text string) int
}

func (m *MockLLMProvider) SendMessage(ctx context.Context, req *LLMRequest) (*LLMResponse, error) {
	if m.sendMessageFunc != nil {
		return m.sendMessageFunc(ctx, req)
	}
	return &LLMResponse{
		Content:    "mock response",
		TokensUsed: 10,
		Duration:   100 * time.Millisecond,
	}, nil
}

func (m *MockLLMProvider) StreamMessage(ctx context.Context, req *LLMRequest) (<-chan *LLMChunk, error) {
	if m.streamMessageFunc != nil {
		return m.streamMessageFunc(ctx, req)
	}
	ch := make(chan *LLMChunk, 2)
	ch <- &LLMChunk{Content: "chunk1", Done: false}
	ch <- &LLMChunk{Content: "chunk2", Done: true}
	close(ch)
	return ch, nil
}

func (m *MockLLMProvider) GetTokenCount(text string) int {
	if m.getTokenCountFunc != nil {
		return m.getTokenCountFunc(text)
	}
	return len(text) / 4 // Simple approximation
}
