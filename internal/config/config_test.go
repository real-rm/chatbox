package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_DefaultValues(t *testing.T) {
	// Clear environment
	clearEnv()
	
	// Set minimum required values
	os.Setenv("JWT_SECRET", "test-secret")
	os.Setenv("S3_ACCESS_KEY_ID", "test-key")
	os.Setenv("S3_SECRET_ACCESS_KEY", "test-secret-key")
	os.Setenv("LLM_PROVIDER_1_ID", "test-provider")
	os.Setenv("LLM_PROVIDER_1_NAME", "Test Provider")
	os.Setenv("LLM_PROVIDER_1_TYPE", "openai")
	os.Setenv("LLM_PROVIDER_1_ENDPOINT", "https://api.test.com")
	os.Setenv("LLM_PROVIDER_1_API_KEY", "test-api-key")
	defer clearEnv()

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Check default values
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, 15*time.Minute, cfg.Server.ReconnectTimeout)
	assert.Equal(t, 10000, cfg.Server.MaxConnections)
	assert.Equal(t, 100, cfg.Server.RateLimit)
	assert.Equal(t, "mongodb://localhost:27017", cfg.Database.URI)
	assert.Equal(t, "chat", cfg.Database.Database)
	assert.Equal(t, "sessions", cfg.Database.Collection)
}

func TestLoad_EnvironmentVariables(t *testing.T) {
	clearEnv()
	
	// Set custom values
	os.Setenv("SERVER_PORT", "9090")
	os.Setenv("RECONNECT_TIMEOUT", "30m")
	os.Setenv("MAX_CONNECTIONS", "5000")
	os.Setenv("RATE_LIMIT", "50")
	os.Setenv("JWT_SECRET", "custom-secret")
	os.Setenv("MONGO_URI", "mongodb://custom:27017")
	os.Setenv("MONGO_DATABASE", "custom_db")
	os.Setenv("MONGO_COLLECTION", "custom_collection")
	os.Setenv("S3_REGION", "eu-west-1")
	os.Setenv("S3_BUCKET", "custom-bucket")
	os.Setenv("S3_ACCESS_KEY_ID", "custom-key")
	os.Setenv("S3_SECRET_ACCESS_KEY", "custom-secret")
	os.Setenv("LLM_PROVIDER_1_ID", "test-provider")
	os.Setenv("LLM_PROVIDER_1_NAME", "Test Provider")
	os.Setenv("LLM_PROVIDER_1_TYPE", "openai")
	os.Setenv("LLM_PROVIDER_1_ENDPOINT", "https://api.test.com")
	os.Setenv("LLM_PROVIDER_1_API_KEY", "test-api-key")
	defer clearEnv()

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, 9090, cfg.Server.Port)
	assert.Equal(t, 30*time.Minute, cfg.Server.ReconnectTimeout)
	assert.Equal(t, 5000, cfg.Server.MaxConnections)
	assert.Equal(t, 50, cfg.Server.RateLimit)
	assert.Equal(t, "custom-secret", cfg.Server.JWTSecret)
	assert.Equal(t, "mongodb://custom:27017", cfg.Database.URI)
	assert.Equal(t, "custom_db", cfg.Database.Database)
	assert.Equal(t, "custom_collection", cfg.Database.Collection)
	assert.Equal(t, "eu-west-1", cfg.Storage.Region)
	assert.Equal(t, "custom-bucket", cfg.Storage.Bucket)
}

func TestLoad_LLMProviders(t *testing.T) {
	clearEnv()
	
	// Set minimum required values
	os.Setenv("JWT_SECRET", "test-secret")
	os.Setenv("S3_ACCESS_KEY_ID", "test-key")
	os.Setenv("S3_SECRET_ACCESS_KEY", "test-secret-key")
	
	// Set multiple LLM providers
	os.Setenv("LLM_PROVIDER_1_ID", "openai-gpt4")
	os.Setenv("LLM_PROVIDER_1_NAME", "GPT-4")
	os.Setenv("LLM_PROVIDER_1_TYPE", "openai")
	os.Setenv("LLM_PROVIDER_1_ENDPOINT", "https://api.openai.com/v1")
	os.Setenv("LLM_PROVIDER_1_API_KEY", "openai-key")
	os.Setenv("LLM_PROVIDER_1_MODEL", "gpt-4")
	
	os.Setenv("LLM_PROVIDER_2_ID", "anthropic-claude")
	os.Setenv("LLM_PROVIDER_2_NAME", "Claude 3")
	os.Setenv("LLM_PROVIDER_2_TYPE", "anthropic")
	os.Setenv("LLM_PROVIDER_2_ENDPOINT", "https://api.anthropic.com/v1")
	os.Setenv("LLM_PROVIDER_2_API_KEY", "anthropic-key")
	os.Setenv("LLM_PROVIDER_2_MODEL", "claude-3-opus")
	defer clearEnv()

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Len(t, cfg.LLM.Providers, 2)
	
	assert.Equal(t, "openai-gpt4", cfg.LLM.Providers[0].ID)
	assert.Equal(t, "GPT-4", cfg.LLM.Providers[0].Name)
	assert.Equal(t, "openai", cfg.LLM.Providers[0].Type)
	assert.Equal(t, "https://api.openai.com/v1", cfg.LLM.Providers[0].Endpoint)
	assert.Equal(t, "openai-key", cfg.LLM.Providers[0].APIKey)
	assert.Equal(t, "gpt-4", cfg.LLM.Providers[0].Model)
	
	assert.Equal(t, "anthropic-claude", cfg.LLM.Providers[1].ID)
	assert.Equal(t, "Claude 3", cfg.LLM.Providers[1].Name)
	assert.Equal(t, "anthropic", cfg.LLM.Providers[1].Type)
}

func TestValidate_ValidConfig(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			Port:             8080,
			ReconnectTimeout: 15 * time.Minute,
			MaxConnections:   10000,
			RateLimit:        100,
			JWTSecret:        "test-secret",
		},
		Database: DatabaseConfig{
			URI:        "mongodb://localhost:27017",
			Database:   "chat",
			Collection: "sessions",
		},
		Storage: StorageConfig{
			Region:          "us-east-1",
			Bucket:          "chat-files",
			AccessKeyID:     "test-key",
			SecretAccessKey: "test-secret",
		},
		LLM: LLMConfig{
			Providers: []LLMProviderConfig{
				{
					ID:       "test-provider",
					Name:     "Test Provider",
					Type:     "openai",
					Endpoint: "https://api.test.com",
					APIKey:   "test-key",
				},
			},
		},
	}

	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestValidate_InvalidServerPort(t *testing.T) {
	tests := []struct {
		name string
		port int
	}{
		{"zero port", 0},
		{"negative port", -1},
		{"port too large", 70000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.Server.Port = tt.port

			err := cfg.Validate()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "port")
		})
	}
}

func TestValidate_MissingJWTSecret(t *testing.T) {
	cfg := validConfig()
	cfg.Server.JWTSecret = ""

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "JWT secret")
}

func TestValidate_InvalidReconnectTimeout(t *testing.T) {
	cfg := validConfig()
	cfg.Server.ReconnectTimeout = 0

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reconnect timeout")
}

func TestValidate_InvalidMaxConnections(t *testing.T) {
	cfg := validConfig()
	cfg.Server.MaxConnections = 0

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "max connections")
}

func TestValidate_InvalidRateLimit(t *testing.T) {
	cfg := validConfig()
	cfg.Server.RateLimit = -1

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "rate limit")
}

func TestValidate_MissingDatabaseURI(t *testing.T) {
	cfg := validConfig()
	cfg.Database.URI = ""

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database URI")
}

func TestValidate_MissingDatabaseName(t *testing.T) {
	cfg := validConfig()
	cfg.Database.Database = ""

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database name")
}

func TestValidate_MissingDatabaseCollection(t *testing.T) {
	cfg := validConfig()
	cfg.Database.Collection = ""

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "collection")
}

func TestValidate_MissingStorageBucket(t *testing.T) {
	cfg := validConfig()
	cfg.Storage.Bucket = ""

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "bucket")
}

func TestValidate_MissingStorageRegion(t *testing.T) {
	cfg := validConfig()
	cfg.Storage.Region = ""

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "region")
}

func TestValidate_MissingStorageCredentials(t *testing.T) {
	tests := []struct {
		name  string
		field string
		value string
	}{
		{"missing access key", "AccessKeyID", ""},
		{"missing secret key", "SecretAccessKey", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			switch tt.field {
			case "AccessKeyID":
				cfg.Storage.AccessKeyID = tt.value
			case "SecretAccessKey":
				cfg.Storage.SecretAccessKey = tt.value
			}

			err := cfg.Validate()
			assert.Error(t, err)
		})
	}
}

func TestValidate_NoLLMProviders(t *testing.T) {
	cfg := validConfig()
	cfg.LLM.Providers = []LLMProviderConfig{}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least one LLM provider")
}

func TestValidate_InvalidLLMProvider(t *testing.T) {
	tests := []struct {
		name     string
		provider LLMProviderConfig
		errMsg   string
	}{
		{
			name:     "missing ID",
			provider: LLMProviderConfig{Name: "Test", Type: "openai", Endpoint: "https://api.test.com", APIKey: "key"},
			errMsg:   "ID is required",
		},
		{
			name:     "missing name",
			provider: LLMProviderConfig{ID: "test", Type: "openai", Endpoint: "https://api.test.com", APIKey: "key"},
			errMsg:   "name is required",
		},
		{
			name:     "missing type",
			provider: LLMProviderConfig{ID: "test", Name: "Test", Endpoint: "https://api.test.com", APIKey: "key"},
			errMsg:   "type is required",
		},
		{
			name:     "invalid type",
			provider: LLMProviderConfig{ID: "test", Name: "Test", Type: "invalid", Endpoint: "https://api.test.com", APIKey: "key"},
			errMsg:   "type must be openai, anthropic, or dify",
		},
		{
			name:     "missing endpoint",
			provider: LLMProviderConfig{ID: "test", Name: "Test", Type: "openai", APIKey: "key"},
			errMsg:   "endpoint is required",
		},
		{
			name:     "missing API key",
			provider: LLMProviderConfig{ID: "test", Name: "Test", Type: "openai", Endpoint: "https://api.test.com"},
			errMsg:   "API key is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.LLM.Providers = []LLMProviderConfig{tt.provider}

			err := cfg.Validate()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.errMsg)
		})
	}
}

func TestGetEnvAsSlice(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected []string
	}{
		{"empty string", "", []string{}},
		{"single value", "value1", []string{"value1"}},
		{"multiple values", "value1,value2,value3", []string{"value1", "value2", "value3"}},
		{"values with spaces", "value1, value2, value3", []string{"value1", " value2", " value3"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("TEST_SLICE", tt.envValue)
			defer os.Unsetenv("TEST_SLICE")

			result := getEnvAsSlice("TEST_SLICE", []string{})
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetEnvAsInt_InvalidValue(t *testing.T) {
	tests := []struct {
		name         string
		envValue     string
		defaultValue int
		expected     int
	}{
		{"invalid integer", "not-a-number", 100, 100},
		{"empty string", "", 50, 50},
		{"valid integer", "200", 100, 200},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv("TEST_INT", tt.envValue)
				defer os.Unsetenv("TEST_INT")
			}

			result := getEnvAsInt("TEST_INT", tt.defaultValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetEnvAsBool_InvalidValue(t *testing.T) {
	tests := []struct {
		name         string
		envValue     string
		defaultValue bool
		expected     bool
	}{
		{"invalid boolean", "not-a-bool", true, true},
		{"empty string", "", false, false},
		{"valid true", "true", false, true},
		{"valid false", "false", true, false},
		{"valid 1", "1", false, true},
		{"valid 0", "0", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv("TEST_BOOL", tt.envValue)
				defer os.Unsetenv("TEST_BOOL")
			} else {
				os.Unsetenv("TEST_BOOL")
			}

			result := getEnvAsBool("TEST_BOOL", tt.defaultValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetEnvAsDuration_InvalidValue(t *testing.T) {
	tests := []struct {
		name         string
		envValue     string
		defaultValue time.Duration
		expected     time.Duration
	}{
		{"invalid duration", "not-a-duration", 5 * time.Minute, 5 * time.Minute},
		{"empty string", "", 10 * time.Second, 10 * time.Second},
		{"valid duration", "30s", 1 * time.Minute, 30 * time.Second},
		{"valid duration with minutes", "15m", 5 * time.Minute, 15 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv("TEST_DURATION", tt.envValue)
				defer os.Unsetenv("TEST_DURATION")
			} else {
				os.Unsetenv("TEST_DURATION")
			}

			result := getEnvAsDuration("TEST_DURATION", tt.defaultValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLoad_KubernetesConfig(t *testing.T) {
	clearEnv()
	
	// Set minimum required values
	os.Setenv("JWT_SECRET", "test-secret")
	os.Setenv("S3_ACCESS_KEY_ID", "test-key")
	os.Setenv("S3_SECRET_ACCESS_KEY", "test-secret-key")
	os.Setenv("LLM_PROVIDER_1_ID", "test-provider")
	os.Setenv("LLM_PROVIDER_1_NAME", "Test Provider")
	os.Setenv("LLM_PROVIDER_1_TYPE", "openai")
	os.Setenv("LLM_PROVIDER_1_ENDPOINT", "https://api.test.com")
	os.Setenv("LLM_PROVIDER_1_API_KEY", "test-api-key")
	
	// Set Kubernetes config
	os.Setenv("K8S_NAMESPACE", "production")
	os.Setenv("K8S_SERVICE_NAME", "chat-service")
	os.Setenv("K8S_CONFIGMAP_NAME", "chat-configmap")
	os.Setenv("K8S_SECRET_NAME", "chat-secret")
	os.Setenv("K8S_ENABLE_PROBE", "false")
	defer clearEnv()

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, "production", cfg.Kubernetes.Namespace)
	assert.Equal(t, "chat-service", cfg.Kubernetes.ServiceName)
	assert.Equal(t, "chat-configmap", cfg.Kubernetes.ConfigMapName)
	assert.Equal(t, "chat-secret", cfg.Kubernetes.SecretName)
	assert.False(t, cfg.Kubernetes.EnableK8sProbe)
}

func TestLoad_NotificationConfig(t *testing.T) {
	clearEnv()
	
	// Set minimum required values
	os.Setenv("JWT_SECRET", "test-secret")
	os.Setenv("S3_ACCESS_KEY_ID", "test-key")
	os.Setenv("S3_SECRET_ACCESS_KEY", "test-secret-key")
	os.Setenv("LLM_PROVIDER_1_ID", "test-provider")
	os.Setenv("LLM_PROVIDER_1_NAME", "Test Provider")
	os.Setenv("LLM_PROVIDER_1_TYPE", "openai")
	os.Setenv("LLM_PROVIDER_1_ENDPOINT", "https://api.test.com")
	os.Setenv("LLM_PROVIDER_1_API_KEY", "test-api-key")
	
	// Set notification config
	os.Setenv("ADMIN_EMAILS", "admin1@example.com,admin2@example.com")
	os.Setenv("ADMIN_PHONES", "+1234567890,+0987654321")
	os.Setenv("EMAIL_FROM", "noreply@example.com")
	os.Setenv("SMTP_HOST", "smtp.example.com")
	os.Setenv("SMTP_PORT", "465")
	os.Setenv("SMTP_USER", "smtp-user")
	os.Setenv("SMTP_PASS", "smtp-pass")
	os.Setenv("SMS_PROVIDER", "twilio")
	os.Setenv("SMS_API_KEY", "sms-api-key")
	defer clearEnv()

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, []string{"admin1@example.com", "admin2@example.com"}, cfg.Notification.AdminEmails)
	assert.Equal(t, []string{"+1234567890", "+0987654321"}, cfg.Notification.AdminPhones)
	assert.Equal(t, "noreply@example.com", cfg.Notification.EmailFrom)
	assert.Equal(t, "smtp.example.com", cfg.Notification.SMTPHost)
	assert.Equal(t, 465, cfg.Notification.SMTPPort)
	assert.Equal(t, "smtp-user", cfg.Notification.SMTPUser)
	assert.Equal(t, "smtp-pass", cfg.Notification.SMTPPass)
	assert.Equal(t, "twilio", cfg.Notification.SMSProvider)
	assert.Equal(t, "sms-api-key", cfg.Notification.SMSAPIKey)
}

func TestLoad_AllLLMProviderTypes(t *testing.T) {
	clearEnv()
	
	// Set minimum required values
	os.Setenv("JWT_SECRET", "test-secret")
	os.Setenv("S3_ACCESS_KEY_ID", "test-key")
	os.Setenv("S3_SECRET_ACCESS_KEY", "test-secret-key")
	
	// Set all three LLM provider types
	os.Setenv("LLM_PROVIDER_1_ID", "openai-provider")
	os.Setenv("LLM_PROVIDER_1_NAME", "OpenAI")
	os.Setenv("LLM_PROVIDER_1_TYPE", "openai")
	os.Setenv("LLM_PROVIDER_1_ENDPOINT", "https://api.openai.com")
	os.Setenv("LLM_PROVIDER_1_API_KEY", "openai-key")
	
	os.Setenv("LLM_PROVIDER_2_ID", "anthropic-provider")
	os.Setenv("LLM_PROVIDER_2_NAME", "Anthropic")
	os.Setenv("LLM_PROVIDER_2_TYPE", "anthropic")
	os.Setenv("LLM_PROVIDER_2_ENDPOINT", "https://api.anthropic.com")
	os.Setenv("LLM_PROVIDER_2_API_KEY", "anthropic-key")
	
	os.Setenv("LLM_PROVIDER_3_ID", "dify-provider")
	os.Setenv("LLM_PROVIDER_3_NAME", "Dify")
	os.Setenv("LLM_PROVIDER_3_TYPE", "dify")
	os.Setenv("LLM_PROVIDER_3_ENDPOINT", "https://api.dify.ai")
	os.Setenv("LLM_PROVIDER_3_API_KEY", "dify-key")
	defer clearEnv()

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Len(t, cfg.LLM.Providers, 3)
	assert.Equal(t, "openai", cfg.LLM.Providers[0].Type)
	assert.Equal(t, "anthropic", cfg.LLM.Providers[1].Type)
	assert.Equal(t, "dify", cfg.LLM.Providers[2].Type)
}

func TestValidate_MultipleErrors(t *testing.T) {
	// Test that multiple validation errors are collected
	cfg := &Config{
		Server: ServerConfig{
			Port:             0, // Invalid
			ReconnectTimeout: 0, // Invalid
			MaxConnections:   0, // Invalid
			RateLimit:        0, // Invalid
			JWTSecret:        "", // Invalid
		},
		Database: DatabaseConfig{
			URI:        "", // Invalid
			Database:   "", // Invalid
			Collection: "", // Invalid
		},
		Storage: StorageConfig{
			Region:          "", // Invalid
			Bucket:          "", // Invalid
			AccessKeyID:     "", // Invalid
			SecretAccessKey: "", // Invalid
		},
		LLM: LLMConfig{
			Providers: []LLMProviderConfig{}, // Invalid
		},
	}

	err := cfg.Validate()
	assert.Error(t, err)
	
	// Check that error message contains multiple validation failures
	errMsg := err.Error()
	assert.Contains(t, errMsg, "port")
	assert.Contains(t, errMsg, "JWT secret")
	assert.Contains(t, errMsg, "reconnect timeout")
	assert.Contains(t, errMsg, "max connections")
	assert.Contains(t, errMsg, "rate limit")
	assert.Contains(t, errMsg, "database URI")
	assert.Contains(t, errMsg, "database name")
	assert.Contains(t, errMsg, "collection")
	assert.Contains(t, errMsg, "bucket")
	assert.Contains(t, errMsg, "region")
	assert.Contains(t, errMsg, "at least one LLM provider")
}

func TestLoad_EmptyLLMProviderSkipped(t *testing.T) {
	clearEnv()
	
	// Set minimum required values
	os.Setenv("JWT_SECRET", "test-secret")
	os.Setenv("S3_ACCESS_KEY_ID", "test-key")
	os.Setenv("S3_SECRET_ACCESS_KEY", "test-secret-key")
	
	// Set provider 1 and 3, skip provider 2
	os.Setenv("LLM_PROVIDER_1_ID", "provider1")
	os.Setenv("LLM_PROVIDER_1_NAME", "Provider 1")
	os.Setenv("LLM_PROVIDER_1_TYPE", "openai")
	os.Setenv("LLM_PROVIDER_1_ENDPOINT", "https://api1.com")
	os.Setenv("LLM_PROVIDER_1_API_KEY", "key1")
	
	// Provider 2 is not set (ID is empty)
	
	os.Setenv("LLM_PROVIDER_3_ID", "provider3")
	os.Setenv("LLM_PROVIDER_3_NAME", "Provider 3")
	os.Setenv("LLM_PROVIDER_3_TYPE", "dify")
	os.Setenv("LLM_PROVIDER_3_ENDPOINT", "https://api3.com")
	os.Setenv("LLM_PROVIDER_3_API_KEY", "key3")
	defer clearEnv()

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Should load both provider 1 and provider 3 (the loop continues after skipping empty IDs)
	assert.Len(t, cfg.LLM.Providers, 2)
	assert.Equal(t, "provider1", cfg.LLM.Providers[0].ID)
	assert.Equal(t, "provider3", cfg.LLM.Providers[1].ID)
}

func TestLoad_DatabaseConnectTimeout(t *testing.T) {
	clearEnv()
	
	// Set minimum required values
	os.Setenv("JWT_SECRET", "test-secret")
	os.Setenv("S3_ACCESS_KEY_ID", "test-key")
	os.Setenv("S3_SECRET_ACCESS_KEY", "test-secret-key")
	os.Setenv("LLM_PROVIDER_1_ID", "test-provider")
	os.Setenv("LLM_PROVIDER_1_NAME", "Test Provider")
	os.Setenv("LLM_PROVIDER_1_TYPE", "openai")
	os.Setenv("LLM_PROVIDER_1_ENDPOINT", "https://api.test.com")
	os.Setenv("LLM_PROVIDER_1_API_KEY", "test-api-key")
	
	// Set custom database timeout
	os.Setenv("MONGO_CONNECT_TIMEOUT", "30s")
	defer clearEnv()

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, 30*time.Second, cfg.Database.ConnectTimeout)
}

// Helper functions

func clearEnv() {
	envVars := []string{
		"SERVER_PORT", "RECONNECT_TIMEOUT", "MAX_CONNECTIONS", "RATE_LIMIT", "JWT_SECRET",
		"MONGO_URI", "MONGO_DATABASE", "MONGO_COLLECTION", "MONGO_CONNECT_TIMEOUT",
		"S3_ENDPOINT", "S3_REGION", "S3_BUCKET", "S3_ACCESS_KEY_ID", "S3_SECRET_ACCESS_KEY",
		"ADMIN_EMAILS", "ADMIN_PHONES", "EMAIL_FROM", "SMTP_HOST", "SMTP_PORT",
		"SMTP_USER", "SMTP_PASS", "SMS_PROVIDER", "SMS_API_KEY",
		"K8S_NAMESPACE", "K8S_SERVICE_NAME", "K8S_CONFIGMAP_NAME", "K8S_SECRET_NAME", "K8S_ENABLE_PROBE",
	}
	
	for i := 1; i <= 10; i++ {
		prefix := "LLM_PROVIDER_" + string(rune('0'+i)) + "_"
		envVars = append(envVars, prefix+"ID", prefix+"NAME", prefix+"TYPE", prefix+"ENDPOINT", prefix+"API_KEY", prefix+"MODEL")
	}
	
	for _, v := range envVars {
		os.Unsetenv(v)
	}
}

func validConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port:             8080,
			ReconnectTimeout: 15 * time.Minute,
			MaxConnections:   10000,
			RateLimit:        100,
			JWTSecret:        "test-secret",
		},
		Database: DatabaseConfig{
			URI:        "mongodb://localhost:27017",
			Database:   "chat",
			Collection: "sessions",
		},
		Storage: StorageConfig{
			Region:          "us-east-1",
			Bucket:          "chat-files",
			AccessKeyID:     "test-key",
			SecretAccessKey: "test-secret",
		},
		LLM: LLMConfig{
			Providers: []LLMProviderConfig{
				{
					ID:       "test-provider",
					Name:     "Test Provider",
					Type:     "openai",
					Endpoint: "https://api.test.com",
					APIKey:   "test-key",
				},
			},
		},
	}
}
