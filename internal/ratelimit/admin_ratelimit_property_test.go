package ratelimit

import (
	"testing"
	"testing/quick"
	"time"
)

// Feature: production-readiness-fixes, Property 19: Admin endpoints enforce rate limits
// **Validates: Requirements 18.1, 18.3**
//
// Property: For any sequence of admin requests exceeding the rate limit,
// requests should be rejected (Allow returns false)
func TestProperty_AdminRateLimitEnforcement(t *testing.T) {
	property := func(userID string, extraRequests uint8) bool {
		if userID == "" {
			userID = "admin-user-1"
		}

		// Create limiter with low limit for testing (5 requests per minute)
		limiter := NewMessageLimiter(1*time.Minute, 5)

		// Make requests up to the limit - all should succeed
		for i := 0; i < 5; i++ {
			if !limiter.Allow(userID) {
				t.Logf("Request %d failed but should have succeeded", i+1)
				return false
			}
		}

		// Make additional requests beyond the limit
		// At least one should fail (we test with 1-10 extra requests)
		numExtra := int(extraRequests%10) + 1
		failedCount := 0

		for i := 0; i < numExtra; i++ {
			if !limiter.Allow(userID) {
				failedCount++
			}
		}

		// At least one request beyond the limit should have failed
		if failedCount == 0 {
			t.Logf("All %d extra requests succeeded, but at least one should have failed", numExtra)
			return false
		}

		return true
	}

	config := &quick.Config{
		MaxCount: 100,
	}

	if err := quick.Check(property, config); err != nil {
		t.Errorf("Property violated: %v", err)
	}
}

// Feature: production-readiness-fixes, Property 19: Admin endpoints enforce rate limits
// **Validates: Requirements 18.1, 18.3**
//
// Property: When rate limit is exceeded, GetRetryAfter should return a positive value
func TestProperty_AdminRateLimitRetryAfter(t *testing.T) {
	property := func(userID string) bool {
		if userID == "" {
			userID = "admin-user-2"
		}

		// Create limiter with low limit for testing
		limiter := NewMessageLimiter(1*time.Minute, 3)

		// Exhaust the limit
		for i := 0; i < 3; i++ {
			limiter.Allow(userID)
		}

		// Next request should fail
		if limiter.Allow(userID) {
			t.Logf("Request succeeded after limit exhausted")
			return false
		}

		// GetRetryAfter should return a positive value
		retryAfter := limiter.GetRetryAfter(userID)
		if retryAfter <= 0 {
			t.Logf("GetRetryAfter returned %d, expected positive value", retryAfter)
			return false
		}

		// RetryAfter should be less than or equal to the window duration
		if retryAfter > int(1*time.Minute.Milliseconds()) {
			t.Logf("GetRetryAfter returned %d ms, which exceeds window of 60000 ms", retryAfter)
			return false
		}

		return true
	}

	config := &quick.Config{
		MaxCount: 100,
	}

	if err := quick.Check(property, config); err != nil {
		t.Errorf("Property violated: %v", err)
	}
}

// Feature: production-readiness-fixes, Property 20: Admin and user limits are independent
// **Validates: Requirements 18.5**
//
// Property: For any user making both admin and user requests,
// the rate limits should be tracked separately (using separate limiter instances)
func TestProperty_IndependentRateLimits(t *testing.T) {
	property := func(userID string) bool {
		if userID == "" {
			userID = "test-user"
		}

		// Create separate limiters for admin and user endpoints
		adminLimiter := NewMessageLimiter(1*time.Minute, 5)
		userLimiter := NewMessageLimiter(1*time.Minute, 10)

		// Exhaust admin limit
		for i := 0; i < 5; i++ {
			if !adminLimiter.Allow(userID) {
				t.Logf("Admin request %d failed unexpectedly", i+1)
				return false
			}
		}

		// Admin limit should be exhausted
		if adminLimiter.Allow(userID) {
			t.Logf("Admin limiter allowed request after limit exhausted")
			return false
		}

		// User limiter should still allow requests (independent tracking)
		for i := 0; i < 10; i++ {
			if !userLimiter.Allow(userID) {
				t.Logf("User request %d failed, but admin limit shouldn't affect user limit", i+1)
				return false
			}
		}

		// User limit should now be exhausted
		if userLimiter.Allow(userID) {
			t.Logf("User limiter allowed request after limit exhausted")
			return false
		}

		// Both limiters should be independent - exhausting one doesn't affect the other
		return true
	}

	config := &quick.Config{
		MaxCount: 100,
	}

	if err := quick.Check(property, config); err != nil {
		t.Errorf("Property violated: %v", err)
	}
}
