package storage

import (
	"context"
	"errors"
	"testing"
	"testing/quick"
	"time"

	"github.com/real-rm/golog"
)

// Feature: production-readiness-fixes, Property 6: Transient errors are retried
// **Validates: Requirements 9.1, 9.3**
//
// Property: For any MongoDB operation that fails with a transient error,
// the operation should be retried up to the maximum attempts
func TestProperty_TransientErrorRetry(t *testing.T) {
	property := func(failCount uint8) bool {
		// Limit fail count to 1-5 attempts
		numFails := int(failCount%5) + 1
		
		// Create a mock operation that fails with transient error N times, then succeeds
		attemptCount := 0
		operation := func() error {
			attemptCount++
			if attemptCount <= numFails {
				return errors.New("connection timeout") // Transient error
			}
			return nil // Success
		}
		
		// Create a minimal storage service for testing with a logger
		logger, err := golog.InitLog(golog.LogConfig{
			Level:          "error",
			StandardOutput: false,
			Dir:            t.TempDir(),
			InfoFile:       "info.log",
			WarnFile:       "warn.log",
			ErrorFile:      "error.log",
		})
		if err != nil {
			t.Logf("Failed to create logger: %v", err)
			return false
		}
		defer logger.Close()
		
		s := &StorageService{
			logger: logger,
		}
		
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		
		// Execute with retry
		err = s.retryOperation(ctx, "TestOperation", operation)
		
		// If numFails < maxAttempts, operation should eventually succeed
		if numFails < defaultRetryConfig.maxAttempts {
			if err != nil {
				t.Logf("Operation failed after %d attempts, but should have succeeded (numFails=%d, maxAttempts=%d)",
					attemptCount, numFails, defaultRetryConfig.maxAttempts)
				return false
			}
			
			// Verify it retried the correct number of times
			if attemptCount != numFails+1 {
				t.Logf("Expected %d attempts, got %d", numFails+1, attemptCount)
				return false
			}
		} else {
			// If numFails >= maxAttempts, operation should fail
			if err == nil {
				t.Logf("Operation succeeded, but should have failed (numFails=%d >= maxAttempts=%d)",
					numFails, defaultRetryConfig.maxAttempts)
				return false
			}
			
			// Verify it attempted maxAttempts times
			if attemptCount != defaultRetryConfig.maxAttempts {
				t.Logf("Expected %d attempts, got %d", defaultRetryConfig.maxAttempts, attemptCount)
				return false
			}
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

// Feature: production-readiness-fixes, Property 7: Retry uses exponential backoff
// **Validates: Requirements 9.2**
//
// Property: For any sequence of retry attempts, the delay between attempts
// should increase exponentially up to the maximum delay
func TestProperty_ExponentialBackoff(t *testing.T) {
	property := func(seed uint8) bool {
		// Track delays between attempts
		var delays []time.Duration
		var lastTime time.Time
		firstCall := true
		
		// Create operation that always fails with transient error
		operation := func() error {
			now := time.Now()
			if !firstCall {
				delays = append(delays, now.Sub(lastTime))
			}
			firstCall = false
			lastTime = now
			return errors.New("connection refused") // Transient error
		}
		
		// Create a minimal storage service for testing with a logger
		logger, err := golog.InitLog(golog.LogConfig{
			Level:          "error",
			StandardOutput: false,
			Dir:            t.TempDir(),
			InfoFile:       "info.log",
			WarnFile:       "warn.log",
			ErrorFile:      "error.log",
		})
		if err != nil {
			t.Logf("Failed to create logger: %v", err)
			return false
		}
		defer logger.Close()
		
		s := &StorageService{
			logger: logger,
		}
		
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		
		// Execute with retry (will fail after max attempts)
		s.retryOperation(ctx, "TestOperation", operation)
		
		// Should have maxAttempts - 1 delays (no delay before first attempt)
		expectedDelays := defaultRetryConfig.maxAttempts - 1
		if len(delays) != expectedDelays {
			t.Logf("Expected %d delays, got %d", expectedDelays, len(delays))
			return false
		}
		
		// Verify exponential backoff
		expectedDelay := defaultRetryConfig.initialDelay
		for i, delay := range delays {
			// Allow 50ms tolerance for timing variations
			tolerance := 50 * time.Millisecond
			
			if delay < expectedDelay-tolerance {
				t.Logf("Delay %d is %v, expected at least %v", i, delay, expectedDelay-tolerance)
				return false
			}
			
			// Calculate next expected delay
			expectedDelay = time.Duration(float64(expectedDelay) * defaultRetryConfig.multiplier)
			if expectedDelay > defaultRetryConfig.maxDelay {
				expectedDelay = defaultRetryConfig.maxDelay
			}
		}
		
		return true
	}
	
	config := &quick.Config{
		MaxCount: 20, // Fewer iterations since this test involves timing
	}
	
	if err := quick.Check(property, config); err != nil {
		t.Errorf("Property violated: %v", err)
	}
}

// Feature: production-readiness-fixes, Property 8: Non-transient errors fail immediately
// **Validates: Requirements 9.1**
//
// Property: For any MongoDB operation that fails with a non-transient error,
// the operation should fail immediately without retries
func TestProperty_NonTransientErrorFailsImmediately(t *testing.T) {
	property := func(seed uint8) bool {
		// Track number of attempts
		attemptCount := 0
		
		// Create operation that fails with non-transient error
		operation := func() error {
			attemptCount++
			return errors.New("duplicate key error") // Non-transient error
		}
		
		// Create a minimal storage service for testing with a logger
		logger, err := golog.InitLog(golog.LogConfig{
			Level:          "error",
			StandardOutput: false,
			Dir:            t.TempDir(),
			InfoFile:       "info.log",
			WarnFile:       "warn.log",
			ErrorFile:      "error.log",
		})
		if err != nil {
			t.Logf("Failed to create logger: %v", err)
			return false
		}
		defer logger.Close()
		
		s := &StorageService{
			logger: logger,
		}
		
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		
		// Execute with retry
		err = s.retryOperation(ctx, "TestOperation", operation)
		
		// Should fail immediately
		if err == nil {
			t.Logf("Operation succeeded, but should have failed")
			return false
		}
		
		// Should only attempt once (no retries for non-transient errors)
		if attemptCount != 1 {
			t.Logf("Expected 1 attempt, got %d", attemptCount)
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
