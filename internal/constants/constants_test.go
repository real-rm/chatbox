package constants

import (
	"testing"
)

func TestTimeoutInvariants(t *testing.T) {
	timeouts := map[string]int64{
		"DefaultContextTimeout":   int64(DefaultContextTimeout),
		"LongContextTimeout":      int64(LongContextTimeout),
		"DefaultLLMStreamTimeout": int64(DefaultLLMStreamTimeout),
		"MongoIndexTimeout":       int64(MongoIndexTimeout),
		"ShortTimeout":            int64(ShortTimeout),
		"MessageAddTimeout":       int64(MessageAddTimeout),
		"SessionEndTimeout":       int64(SessionEndTimeout),
		"HealthCheckTimeout":      int64(HealthCheckTimeout),
		"MetricsTimeout":          int64(MetricsTimeout),
		"VoiceProcessTimeout":     int64(VoiceProcessTimeout),
		"HTTPReadTimeout":         int64(HTTPReadTimeout),
		"HTTPWriteTimeout":        int64(HTTPWriteTimeout),
		"HTTPIdleTimeout":         int64(HTTPIdleTimeout),
	}

	for name, val := range timeouts {
		if val <= 0 {
			t.Errorf("timeout %s must be positive, got %d", name, val)
		}
	}
}

func TestKeyLengthInvariants(t *testing.T) {
	if EncryptionKeyLength <= 0 {
		t.Errorf("EncryptionKeyLength must be positive, got %d", EncryptionKeyLength)
	}
	if MinJWTSecretLength < 32 {
		t.Errorf("MinJWTSecretLength must be >= 32 for 256-bit security, got %d", MinJWTSecretLength)
	}
}

func TestWeakSecretsNonEmpty(t *testing.T) {
	if len(WeakSecrets) == 0 {
		t.Error("WeakSecrets list must not be empty")
	}
}

func TestLimitsInvariants(t *testing.T) {
	limits := map[string]int{
		"DefaultMaxMessageSize": DefaultMaxMessageSize,
		"DefaultSessionLimit":   DefaultSessionLimit,
		"MaxSessionLimit":       MaxSessionLimit,
		"DefaultRateLimit":      DefaultRateLimit,
		"DefaultAdminRateLimit": DefaultAdminRateLimit,
		"MaxRetryAttempts":      MaxRetryAttempts,
		"PublicEndpointRate":    PublicEndpointRate,
	}

	for name, val := range limits {
		if val <= 0 {
			t.Errorf("limit %s must be positive, got %d", name, val)
		}
	}

	if MaxSessionLimit < DefaultSessionLimit {
		t.Errorf("MaxSessionLimit (%d) must be >= DefaultSessionLimit (%d)", MaxSessionLimit, DefaultSessionLimit)
	}
}

func TestValidSortFieldsNonEmpty(t *testing.T) {
	if len(ValidSortFields) == 0 {
		t.Error("ValidSortFields must not be empty")
	}
}

func TestValidSortOrdersContainsExpected(t *testing.T) {
	if !ValidSortOrders["asc"] {
		t.Error("ValidSortOrders must contain 'asc'")
	}
	if !ValidSortOrders["desc"] {
		t.Error("ValidSortOrders must contain 'desc'")
	}
}

func TestDurationInvariants(t *testing.T) {
	if DefaultReconnectTimeout <= 0 {
		t.Error("DefaultReconnectTimeout must be positive")
	}
	if DefaultRateWindow <= 0 {
		t.Error("DefaultRateWindow must be positive")
	}
	if DefaultCleanupInterval <= 0 {
		t.Error("DefaultCleanupInterval must be positive")
	}
	if DefaultSessionTTL <= 0 {
		t.Error("DefaultSessionTTL must be positive")
	}
	if InitialRetryDelay <= 0 {
		t.Error("InitialRetryDelay must be positive")
	}
	if MaxRetryDelay <= 0 {
		t.Error("MaxRetryDelay must be positive")
	}
	if MaxRetryDelay < InitialRetryDelay {
		t.Errorf("MaxRetryDelay (%v) must be >= InitialRetryDelay (%v)", MaxRetryDelay, InitialRetryDelay)
	}
}
