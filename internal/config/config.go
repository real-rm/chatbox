package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all application configuration
type Config struct {
	Server       ServerConfig
	LLM          LLMConfig
	Database     DatabaseConfig
	Storage      StorageConfig
	Notification NotificationConfig
	Kubernetes   KubernetesConfig
}

// ServerConfig holds server-specific configuration
type ServerConfig struct {
	Port             int
	ReconnectTimeout time.Duration
	MaxConnections   int
	RateLimit        int
	JWTSecret        string
}

// LLMConfig holds LLM provider configurations
type LLMConfig struct {
	Providers []LLMProviderConfig
}

// LLMProviderConfig holds configuration for a single LLM provider
type LLMProviderConfig struct {
	ID       string
	Name     string
	Type     string // "openai", "anthropic", "dify"
	Endpoint string
	APIKey   string
	Model    string
}

// DatabaseConfig holds MongoDB configuration
type DatabaseConfig struct {
	URI            string
	Database       string
	Collection     string
	ConnectTimeout time.Duration
}

// StorageConfig holds S3 storage configuration
type StorageConfig struct {
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
}

// NotificationConfig holds notification service configuration
type NotificationConfig struct {
	AdminEmails []string
	AdminPhones []string
	EmailFrom   string
	SMTPHost    string
	SMTPPort    int
	SMTPUser    string
	SMTPPass    string
	SMSProvider string
	SMSAPIKey   string
	Rules       []NotificationRule
}

// NotificationRule defines when to send notifications
type NotificationRule struct {
	EventType string
	Channels  []string // "email", "sms"
	Enabled   bool
}

// KubernetesConfig holds Kubernetes-specific configuration
type KubernetesConfig struct {
	Namespace      string
	ServiceName    string
	ConfigMapName  string
	SecretName     string
	EnableK8sProbe bool
}

// Load loads configuration from environment variables and Kubernetes ConfigMaps/Secrets
func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port:             getEnvAsInt("SERVER_PORT", 8080),
			ReconnectTimeout: getEnvAsDuration("RECONNECT_TIMEOUT", 15*time.Minute),
			MaxConnections:   getEnvAsInt("MAX_CONNECTIONS", 10000),
			RateLimit:        getEnvAsInt("RATE_LIMIT", 100),
			JWTSecret:        getEnv("JWT_SECRET", ""),
		},
		Database: DatabaseConfig{
			URI:            getEnv("MONGO_URI", "mongodb://localhost:27017"),
			Database:       getEnv("MONGO_DATABASE", "chat"),
			Collection:     getEnv("MONGO_COLLECTION", "sessions"),
			ConnectTimeout: getEnvAsDuration("MONGO_CONNECT_TIMEOUT", 10*time.Second),
		},
		Storage: StorageConfig{
			Endpoint:        getEnv("S3_ENDPOINT", ""),
			Region:          getEnv("S3_REGION", "us-east-1"),
			Bucket:          getEnv("S3_BUCKET", "chat-files"),
			AccessKeyID:     getEnv("S3_ACCESS_KEY_ID", ""),
			SecretAccessKey: getEnv("S3_SECRET_ACCESS_KEY", ""),
		},
		Notification: NotificationConfig{
			AdminEmails: getEnvAsSlice("ADMIN_EMAILS", []string{}),
			AdminPhones: getEnvAsSlice("ADMIN_PHONES", []string{}),
			EmailFrom:   getEnv("EMAIL_FROM", ""),
			SMTPHost:    getEnv("SMTP_HOST", ""),
			SMTPPort:    getEnvAsInt("SMTP_PORT", 587),
			SMTPUser:    getEnv("SMTP_USER", ""),
			SMTPPass:    getEnv("SMTP_PASS", ""),
			SMSProvider: getEnv("SMS_PROVIDER", ""),
			SMSAPIKey:   getEnv("SMS_API_KEY", ""),
			Rules:       []NotificationRule{},
		},
		Kubernetes: KubernetesConfig{
			Namespace:      getEnv("K8S_NAMESPACE", "default"),
			ServiceName:    getEnv("K8S_SERVICE_NAME", "chat-websocket"),
			ConfigMapName:  getEnv("K8S_CONFIGMAP_NAME", "chat-config"),
			SecretName:     getEnv("K8S_SECRET_NAME", "chat-secrets"),
			EnableK8sProbe: getEnvAsBool("K8S_ENABLE_PROBE", true),
		},
	}

	// Load LLM providers from environment
	cfg.LLM = loadLLMConfig()

	return cfg, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	var errs []error

	// Validate server config
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		errs = append(errs, errors.New("server port must be between 1 and 65535"))
	}
	if c.Server.JWTSecret == "" {
		errs = append(errs, errors.New("JWT secret is required"))
	}
	if c.Server.ReconnectTimeout <= 0 {
		errs = append(errs, errors.New("reconnect timeout must be positive"))
	}
	if c.Server.MaxConnections <= 0 {
		errs = append(errs, errors.New("max connections must be positive"))
	}
	if c.Server.RateLimit <= 0 {
		errs = append(errs, errors.New("rate limit must be positive"))
	}

	// Validate database config
	if c.Database.URI == "" {
		errs = append(errs, errors.New("database URI is required"))
	}
	if c.Database.Database == "" {
		errs = append(errs, errors.New("database name is required"))
	}
	if c.Database.Collection == "" {
		errs = append(errs, errors.New("database collection is required"))
	}

	// Validate storage config
	if c.Storage.Bucket == "" {
		errs = append(errs, errors.New("storage bucket is required"))
	}
	if c.Storage.Region == "" {
		errs = append(errs, errors.New("storage region is required"))
	}
	if c.Storage.AccessKeyID == "" {
		errs = append(errs, errors.New("storage access key ID is required"))
	}
	if c.Storage.SecretAccessKey == "" {
		errs = append(errs, errors.New("storage secret access key is required"))
	}

	// Validate LLM config
	if len(c.LLM.Providers) == 0 {
		errs = append(errs, errors.New("at least one LLM provider is required"))
	}
	for i, provider := range c.LLM.Providers {
		if provider.ID == "" {
			errs = append(errs, fmt.Errorf("LLM provider %d: ID is required", i))
		}
		if provider.Name == "" {
			errs = append(errs, fmt.Errorf("LLM provider %d: name is required", i))
		}
		if provider.Type == "" {
			errs = append(errs, fmt.Errorf("LLM provider %d: type is required", i))
		}
		if provider.Type != "openai" && provider.Type != "anthropic" && provider.Type != "dify" {
			errs = append(errs, fmt.Errorf("LLM provider %d: type must be openai, anthropic, or dify", i))
		}
		if provider.Endpoint == "" {
			errs = append(errs, fmt.Errorf("LLM provider %d: endpoint is required", i))
		}
		if provider.APIKey == "" {
			errs = append(errs, fmt.Errorf("LLM provider %d: API key is required", i))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("configuration validation failed: %v", errs)
	}

	return nil
}

// Helper functions for environment variable parsing

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return defaultValue
	}
	return value
}

func getEnvAsBool(key string, defaultValue bool) bool {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	value, err := strconv.ParseBool(valueStr)
	if err != nil {
		return defaultValue
	}
	return value
}

func getEnvAsDuration(key string, defaultValue time.Duration) time.Duration {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	value, err := time.ParseDuration(valueStr)
	if err != nil {
		return defaultValue
	}
	return value
}

func getEnvAsSlice(key string, defaultValue []string) []string {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	// Simple comma-separated parsing
	result := []string{}
	for _, v := range splitByComma(valueStr) {
		if v != "" {
			result = append(result, v)
		}
	}
	return result
}

func splitByComma(s string) []string {
	result := []string{}
	current := ""
	for _, c := range s {
		if c == ',' {
			result = append(result, current)
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

func loadLLMConfig() LLMConfig {
	// Load LLM providers from environment
	// Format: LLM_PROVIDER_1_ID, LLM_PROVIDER_1_NAME, etc.
	providers := []LLMProviderConfig{}
	
	for i := 1; i <= 10; i++ { // Support up to 10 providers
		prefix := fmt.Sprintf("LLM_PROVIDER_%d_", i)
		id := os.Getenv(prefix + "ID")
		if id == "" {
			continue // No more providers
		}
		
		provider := LLMProviderConfig{
			ID:       id,
			Name:     os.Getenv(prefix + "NAME"),
			Type:     os.Getenv(prefix + "TYPE"),
			Endpoint: os.Getenv(prefix + "ENDPOINT"),
			APIKey:   os.Getenv(prefix + "API_KEY"),
			Model:    os.Getenv(prefix + "MODEL"),
		}
		providers = append(providers, provider)
	}
	
	return LLMConfig{
		Providers: providers,
	}
}
