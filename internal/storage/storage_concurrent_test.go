package storage

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/real-rm/chatbox/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConcurrentSessionCreation tests creating multiple sessions concurrently
// Validates: Requirements 6.3
func TestConcurrentSessionCreation(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "chatbox", "sessions", logger, nil)

	// Number of concurrent operations
	numSessions := 50
	now := time.Now()

	// Use WaitGroup to synchronize goroutines
	var wg sync.WaitGroup
	wg.Add(numSessions)

	// Channel to collect errors
	errChan := make(chan error, numSessions)

	// Create sessions concurrently
	for i := 0; i < numSessions; i++ {
		go func(index int) {
			defer wg.Done()

			sess := &session.Session{
				ID:            fmt.Sprintf("concurrent-session-%d", index),
				UserID:        fmt.Sprintf("user-%d", index%10), // 10 different users
				Name:          fmt.Sprintf("Concurrent Session %d", index),
				ModelID:       "gpt-4",
				Messages:      []*session.Message{},
				StartTime:     now,
				LastActivity:  now,
				EndTime:       nil,
				IsActive:      true,
				HelpRequested: false,
				AdminAssisted: false,
				TotalTokens:   0,
				ResponseTimes: []time.Duration{},
			}

			err := service.CreateSession(sess)
			if err != nil {
				errChan <- fmt.Errorf("failed to create session %d: %w", index, err)
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errChan)

	// Check for errors
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}
	assert.Empty(t, errors, "Expected no errors during concurrent session creation")

	// Verify all sessions were created
	for i := 0; i < numSessions; i++ {
		sessionID := fmt.Sprintf("concurrent-session-%d", i)
		sess, err := service.GetSession(sessionID)
		assert.NoError(t, err, "Session %s should exist", sessionID)
		assert.NotNil(t, sess, "Session %s should not be nil", sessionID)
		if sess != nil {
			assert.Equal(t, sessionID, sess.ID)
			assert.Equal(t, fmt.Sprintf("Concurrent Session %d", i), sess.Name)
		}
	}
}

// TestConcurrentMessageAddition tests adding messages to the same session concurrently
// Validates: Requirements 6.3
func TestConcurrentMessageAddition(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "chatbox", "sessions", logger, nil)

	// Create a single session
	now := time.Now()
	sess := &session.Session{
		ID:            "concurrent-msg-session",
		UserID:        "user-concurrent",
		Name:          "Concurrent Message Test",
		ModelID:       "gpt-4",
		Messages:      []*session.Message{},
		StartTime:     now,
		LastActivity:  now,
		EndTime:       nil,
		IsActive:      true,
		HelpRequested: false,
		AdminAssisted: false,
		TotalTokens:   0,
		ResponseTimes: []time.Duration{},
	}

	err := service.CreateSession(sess)
	require.NoError(t, err)

	// Number of concurrent messages
	numMessages := 100

	// Use WaitGroup to synchronize goroutines
	var wg sync.WaitGroup
	wg.Add(numMessages)

	// Channel to collect errors
	errChan := make(chan error, numMessages)

	// Add messages concurrently
	for i := 0; i < numMessages; i++ {
		go func(index int) {
			defer wg.Done()

			msg := &session.Message{
				Content:   fmt.Sprintf("Concurrent message %d", index),
				Timestamp: now.Add(time.Duration(index) * time.Millisecond),
				Sender:    "user",
				FileID:    "",
				FileURL:   "",
				Metadata:  map[string]string{"index": fmt.Sprintf("%d", index)},
			}

			err := service.AddMessage("concurrent-msg-session", msg)
			if err != nil {
				errChan <- fmt.Errorf("failed to add message %d: %w", index, err)
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errChan)

	// Check for errors
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}
	assert.Empty(t, errors, "Expected no errors during concurrent message addition")

	// Verify all messages were added
	retrievedSess, err := service.GetSession("concurrent-msg-session")
	assert.NoError(t, err)
	assert.NotNil(t, retrievedSess)
	assert.Equal(t, numMessages, len(retrievedSess.Messages), "All messages should be added")
}

// TestConcurrentSessionUpdates tests updating the same session concurrently
// Validates: Requirements 6.3
func TestConcurrentSessionUpdates(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "chatbox", "sessions", logger, nil)

	// Create a session
	now := time.Now()
	sess := &session.Session{
		ID:            "concurrent-update-session",
		UserID:        "user-update",
		Name:          "Concurrent Update Test",
		ModelID:       "gpt-4",
		Messages:      []*session.Message{},
		StartTime:     now,
		LastActivity:  now,
		EndTime:       nil,
		IsActive:      true,
		HelpRequested: false,
		AdminAssisted: false,
		TotalTokens:   0,
		ResponseTimes: []time.Duration{},
	}

	err := service.CreateSession(sess)
	require.NoError(t, err)

	// Number of concurrent updates
	numUpdates := 50

	// Use WaitGroup to synchronize goroutines
	var wg sync.WaitGroup
	wg.Add(numUpdates)

	// Channel to collect errors
	errChan := make(chan error, numUpdates)

	// Update session concurrently
	for i := 0; i < numUpdates; i++ {
		go func(index int) {
			defer wg.Done()

			// Get the current session
			currentSess, err := service.GetSession("concurrent-update-session")
			if err != nil {
				errChan <- fmt.Errorf("failed to get session for update %d: %w", index, err)
				return
			}

			// Update session fields
			currentSess.Name = fmt.Sprintf("Updated Name %d", index)
			currentSess.TotalTokens += 10
			currentSess.LastActivity = now.Add(time.Duration(index) * time.Second)

			err = service.UpdateSession(currentSess)
			if err != nil {
				errChan <- fmt.Errorf("failed to update session %d: %w", index, err)
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errChan)

	// Check for errors
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}
	assert.Empty(t, errors, "Expected no errors during concurrent session updates")

	// Verify session still exists and has been updated
	finalSess, err := service.GetSession("concurrent-update-session")
	assert.NoError(t, err)
	assert.NotNil(t, finalSess)
	assert.Equal(t, "concurrent-update-session", finalSess.ID)
	// The final state will be one of the updates (last write wins)
	assert.Greater(t, finalSess.TotalTokens, 0, "TotalTokens should have been updated")
}

// TestConcurrentMixedOperations tests a mix of create, read, update operations concurrently
// Validates: Requirements 6.3
func TestConcurrentMixedOperations(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "chatbox", "sessions", logger, nil)

	now := time.Now()

	// Create initial sessions
	numInitialSessions := 10
	for i := 0; i < numInitialSessions; i++ {
		sess := &session.Session{
			ID:            fmt.Sprintf("mixed-session-%d", i),
			UserID:        fmt.Sprintf("user-%d", i%3),
			Name:          fmt.Sprintf("Mixed Session %d", i),
			ModelID:       "gpt-4",
			Messages:      []*session.Message{},
			StartTime:     now,
			LastActivity:  now,
			EndTime:       nil,
			IsActive:      true,
			HelpRequested: false,
			AdminAssisted: false,
			TotalTokens:   100,
			ResponseTimes: []time.Duration{},
		}
		err := service.CreateSession(sess)
		require.NoError(t, err)
	}

	// Number of concurrent operations
	numOperations := 100

	// Use WaitGroup to synchronize goroutines
	var wg sync.WaitGroup
	wg.Add(numOperations)

	// Channel to collect errors
	errChan := make(chan error, numOperations)

	// Perform mixed operations concurrently
	for i := 0; i < numOperations; i++ {
		go func(index int) {
			defer wg.Done()

			opType := index % 4

			switch opType {
			case 0: // Create new session
				sess := &session.Session{
					ID:            fmt.Sprintf("mixed-new-session-%d", index),
					UserID:        fmt.Sprintf("user-%d", index%3),
					Name:          fmt.Sprintf("New Session %d", index),
					ModelID:       "gpt-4",
					Messages:      []*session.Message{},
					StartTime:     now,
					LastActivity:  now,
					EndTime:       nil,
					IsActive:      true,
					HelpRequested: false,
					AdminAssisted: false,
					TotalTokens:   0,
					ResponseTimes: []time.Duration{},
				}
				err := service.CreateSession(sess)
				if err != nil {
					errChan <- fmt.Errorf("create operation %d failed: %w", index, err)
				}

			case 1: // Read existing session
				sessionID := fmt.Sprintf("mixed-session-%d", index%numInitialSessions)
				_, err := service.GetSession(sessionID)
				if err != nil {
					errChan <- fmt.Errorf("read operation %d failed: %w", index, err)
				}

			case 2: // Update existing session
				sessionID := fmt.Sprintf("mixed-session-%d", index%numInitialSessions)
				sess, err := service.GetSession(sessionID)
				if err != nil {
					errChan <- fmt.Errorf("get for update operation %d failed: %w", index, err)
					return
				}
				sess.TotalTokens += 10
				err = service.UpdateSession(sess)
				if err != nil {
					errChan <- fmt.Errorf("update operation %d failed: %w", index, err)
				}

			case 3: // Add message to existing session
				sessionID := fmt.Sprintf("mixed-session-%d", index%numInitialSessions)
				msg := &session.Message{
					Content:   fmt.Sprintf("Message from operation %d", index),
					Timestamp: now.Add(time.Duration(index) * time.Millisecond),
					Sender:    "user",
					FileID:    "",
					FileURL:   "",
					Metadata:  map[string]string{},
				}
				err := service.AddMessage(sessionID, msg)
				if err != nil {
					errChan <- fmt.Errorf("add message operation %d failed: %w", index, err)
				}
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errChan)

	// Check for errors
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}
	assert.Empty(t, errors, "Expected no errors during concurrent mixed operations")

	// Verify initial sessions still exist and have been modified
	for i := 0; i < numInitialSessions; i++ {
		sessionID := fmt.Sprintf("mixed-session-%d", i)
		sess, err := service.GetSession(sessionID)
		assert.NoError(t, err)
		assert.NotNil(t, sess)
		if sess != nil {
			assert.Equal(t, sessionID, sess.ID)
			// TotalTokens should have increased from updates
			assert.GreaterOrEqual(t, sess.TotalTokens, 100, "TotalTokens should be at least initial value")
			// Messages should have been added
			assert.Greater(t, len(sess.Messages), 0, "Messages should have been added")
		}
	}
}

// TestConcurrentSessionCreationWithEncryption tests concurrent session creation with encryption enabled
// Validates: Requirements 6.3
func TestConcurrentSessionCreationWithEncryption(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	// Create 32-byte encryption key for AES-256
	encryptionKey := []byte("12345678901234567890123456789012")
	service := NewStorageService(mongoClient, "chatbox", "sessions", logger, encryptionKey)

	// Number of concurrent operations
	numSessions := 30
	now := time.Now()

	// Use WaitGroup to synchronize goroutines
	var wg sync.WaitGroup
	wg.Add(numSessions)

	// Channel to collect errors
	errChan := make(chan error, numSessions)

	// Create sessions with messages concurrently
	for i := 0; i < numSessions; i++ {
		go func(index int) {
			defer wg.Done()

			sess := &session.Session{
				ID:      fmt.Sprintf("encrypted-concurrent-session-%d", index),
				UserID:  fmt.Sprintf("user-%d", index%5),
				Name:    fmt.Sprintf("Encrypted Concurrent Session %d", index),
				ModelID: "gpt-4",
				Messages: []*session.Message{
					{
						Content:   fmt.Sprintf("Sensitive message %d", index),
						Timestamp: now,
						Sender:    "user",
						FileID:    "",
						FileURL:   "",
						Metadata:  map[string]string{"sensitive": "true"},
					},
				},
				StartTime:     now,
				LastActivity:  now,
				EndTime:       nil,
				IsActive:      true,
				HelpRequested: false,
				AdminAssisted: false,
				TotalTokens:   0,
				ResponseTimes: []time.Duration{},
			}

			err := service.CreateSession(sess)
			if err != nil {
				errChan <- fmt.Errorf("failed to create encrypted session %d: %w", index, err)
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errChan)

	// Check for errors
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}
	assert.Empty(t, errors, "Expected no errors during concurrent encrypted session creation")

	// Verify all sessions were created and can be decrypted
	for i := 0; i < numSessions; i++ {
		sessionID := fmt.Sprintf("encrypted-concurrent-session-%d", i)
		sess, err := service.GetSession(sessionID)
		assert.NoError(t, err, "Session %s should exist", sessionID)
		assert.NotNil(t, sess, "Session %s should not be nil", sessionID)
		if sess != nil {
			assert.Equal(t, sessionID, sess.ID)
			assert.Len(t, sess.Messages, 1)
			// Verify message was decrypted correctly
			assert.Equal(t, fmt.Sprintf("Sensitive message %d", i), sess.Messages[0].Content)
		}
	}
}

// TestConcurrentListOperations tests concurrent list operations while sessions are being modified
// Validates: Requirements 6.3
func TestConcurrentListOperations(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "chatbox", "sessions", logger, nil)

	now := time.Now()

	// Create initial sessions
	numInitialSessions := 20
	for i := 0; i < numInitialSessions; i++ {
		sess := &session.Session{
			ID:            fmt.Sprintf("list-session-%d", i),
			UserID:        "user-list",
			Name:          fmt.Sprintf("List Session %d", i),
			ModelID:       "gpt-4",
			Messages:      []*session.Message{},
			StartTime:     now.Add(-time.Duration(i) * time.Hour),
			LastActivity:  now,
			EndTime:       nil,
			IsActive:      true,
			HelpRequested: false,
			AdminAssisted: false,
			TotalTokens:   100,
			ResponseTimes: []time.Duration{},
		}
		err := service.CreateSession(sess)
		require.NoError(t, err)
	}

	// Number of concurrent operations
	numOperations := 50

	// Use WaitGroup to synchronize goroutines
	var wg sync.WaitGroup
	wg.Add(numOperations)

	// Channel to collect errors
	errChan := make(chan error, numOperations)

	// Perform concurrent list and modify operations
	for i := 0; i < numOperations; i++ {
		go func(index int) {
			defer wg.Done()

			if index%2 == 0 {
				// List operations
				_, err := service.ListUserSessions("user-list", 10)
				if err != nil {
					errChan <- fmt.Errorf("list operation %d failed: %w", index, err)
				}
			} else {
				// Modify operations (add message)
				sessionID := fmt.Sprintf("list-session-%d", index%numInitialSessions)
				msg := &session.Message{
					Content:   fmt.Sprintf("Message from operation %d", index),
					Timestamp: now.Add(time.Duration(index) * time.Millisecond),
					Sender:    "user",
					FileID:    "",
					FileURL:   "",
					Metadata:  map[string]string{},
				}
				err := service.AddMessage(sessionID, msg)
				if err != nil {
					errChan <- fmt.Errorf("add message operation %d failed: %w", index, err)
				}
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errChan)

	// Check for errors
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}
	assert.Empty(t, errors, "Expected no errors during concurrent list operations")

	// Verify final state
	sessions, err := service.ListUserSessions("user-list", 0)
	assert.NoError(t, err)
	assert.Equal(t, numInitialSessions, len(sessions), "All initial sessions should still exist")
}
