package chatbox

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/real-rm/chatbox/internal/constants"
	"github.com/real-rm/chatbox/internal/ratelimit"
	"github.com/real-rm/golog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublicRateLimitMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	createTestLogger := func(t *testing.T) *golog.Logger {
		t.Helper()
		logger, err := golog.InitLog(golog.LogConfig{
			Dir:            t.TempDir(),
			Level:          "error",
			StandardOutput: false,
		})
		require.NoError(t, err)
		return logger
	}

	t.Run("AllowsRequestsUnderLimit", func(t *testing.T) {
		logger := createTestLogger(t)
		defer logger.Close()

		limiter := ratelimit.NewMessageLimiter(1*time.Minute, 10)
		router := gin.New()
		router.Use(publicRateLimitMiddleware(limiter, logger))
		router.GET("/healthz", handleHealthCheck)

		for i := 0; i < 10; i++ {
			req, _ := http.NewRequest("GET", "/healthz", nil)
			req.RemoteAddr = "192.168.1.1:12345"
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			assert.Equal(t, 200, w.Code, "request %d should succeed", i)
		}
	})

	t.Run("RejectsRequestsOverLimit", func(t *testing.T) {
		logger := createTestLogger(t)
		defer logger.Close()

		limiter := ratelimit.NewMessageLimiter(1*time.Minute, 5)
		router := gin.New()
		router.Use(publicRateLimitMiddleware(limiter, logger))
		router.GET("/healthz", handleHealthCheck)

		// Use up the limit
		for i := 0; i < 5; i++ {
			req, _ := http.NewRequest("GET", "/healthz", nil)
			req.RemoteAddr = "10.0.0.1:12345"
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			assert.Equal(t, 200, w.Code)
		}

		// Next request should be rejected
		req, _ := http.NewRequest("GET", "/healthz", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, constants.StatusTooManyRequests, w.Code)
		assert.Contains(t, w.Body.String(), "rate_limit_exceeded")
	})

	t.Run("DifferentIPsHaveSeparateLimits", func(t *testing.T) {
		logger := createTestLogger(t)
		defer logger.Close()

		limiter := ratelimit.NewMessageLimiter(1*time.Minute, 3)
		router := gin.New()
		router.Use(publicRateLimitMiddleware(limiter, logger))
		router.GET("/healthz", handleHealthCheck)

		// Use up limit for IP1
		for i := 0; i < 3; i++ {
			req, _ := http.NewRequest("GET", "/healthz", nil)
			req.RemoteAddr = "10.0.0.1:12345"
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			assert.Equal(t, 200, w.Code)
		}

		// IP1 should be limited
		req, _ := http.NewRequest("GET", "/healthz", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, constants.StatusTooManyRequests, w.Code)

		// IP2 should still be allowed
		req2, _ := http.NewRequest("GET", "/healthz", nil)
		req2.RemoteAddr = "10.0.0.2:12345"
		w2 := httptest.NewRecorder()
		router.ServeHTTP(w2, req2)
		assert.Equal(t, 200, w2.Code)
	})

	t.Run("UsesXForwardedForHeader", func(t *testing.T) {
		logger := createTestLogger(t)
		defer logger.Close()

		limiter := ratelimit.NewMessageLimiter(1*time.Minute, 2)
		router := gin.New()
		router.Use(publicRateLimitMiddleware(limiter, logger))
		router.GET("/healthz", handleHealthCheck)

		// Send requests with X-Forwarded-For header
		for i := 0; i < 2; i++ {
			req, _ := http.NewRequest("GET", "/healthz", nil)
			req.RemoteAddr = "10.0.0.1:12345"
			req.Header.Set("X-Forwarded-For", "203.0.113.50")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			assert.Equal(t, 200, w.Code)
		}

		// Same forwarded IP should be limited
		req, _ := http.NewRequest("GET", "/healthz", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		req.Header.Set("X-Forwarded-For", "203.0.113.50")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, constants.StatusTooManyRequests, w.Code)
	})

	t.Run("IncludesRetryAfterHeader", func(t *testing.T) {
		logger := createTestLogger(t)
		defer logger.Close()

		limiter := ratelimit.NewMessageLimiter(1*time.Minute, 1)
		router := gin.New()
		router.Use(publicRateLimitMiddleware(limiter, logger))
		router.GET("/healthz", handleHealthCheck)

		// First request succeeds
		req, _ := http.NewRequest("GET", "/healthz", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, 200, w.Code)

		// Second request should be rate limited with Retry-After header
		req2, _ := http.NewRequest("GET", "/healthz", nil)
		req2.RemoteAddr = "10.0.0.1:12345"
		w2 := httptest.NewRecorder()
		router.ServeHTTP(w2, req2)
		assert.Equal(t, constants.StatusTooManyRequests, w2.Code)
		retryAfter := w2.Header().Get(constants.HeaderRetryAfter)
		assert.NotEmpty(t, retryAfter, "should include Retry-After header")
	})
}

func TestPublicRateLimitMiddleware_HighVolume(t *testing.T) {
	gin.SetMode(gin.TestMode)

	logger, err := golog.InitLog(golog.LogConfig{
		Dir:            t.TempDir(),
		Level:          "error",
		StandardOutput: false,
	})
	require.NoError(t, err)
	defer logger.Close()

	limiter := ratelimit.NewMessageLimiter(1*time.Minute, constants.PublicEndpointRate)
	router := gin.New()
	router.Use(publicRateLimitMiddleware(limiter, logger))
	router.GET("/healthz", handleHealthCheck)

	successCount := 0
	rejectCount := 0

	for i := 0; i < constants.PublicEndpointRate+20; i++ {
		req, _ := http.NewRequest("GET", "/healthz", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code == 200 {
			successCount++
		} else if w.Code == constants.StatusTooManyRequests {
			rejectCount++
		}
	}

	assert.Equal(t, constants.PublicEndpointRate, successCount,
		fmt.Sprintf("should allow exactly %d requests", constants.PublicEndpointRate))
	assert.Equal(t, 20, rejectCount, "should reject excess requests")
}
