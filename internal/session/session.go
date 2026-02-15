package session

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrSessionNotFound is returned when a session is not found
	ErrSessionNotFound = errors.New("session not found")
	// ErrInvalidUserID is returned when user ID is empty
	ErrInvalidUserID = errors.New("user ID cannot be empty")
	// ErrInvalidSessionID is returned when session ID is empty
	ErrInvalidSessionID = errors.New("session ID cannot be empty")
	// ErrActiveSessionExists is returned when user already has an active session
	ErrActiveSessionExists = errors.New("user already has an active session")
	// ErrSessionTimeout is returned when trying to restore an expired session
	ErrSessionTimeout = errors.New("session has timed out")
	// ErrSessionOwnership is returned when session doesn't belong to user
	ErrSessionOwnership = errors.New("session does not belong to user")
	// ErrNegativeTokens is returned when trying to add negative tokens
	ErrNegativeTokens = errors.New("token count cannot be negative")
	// ErrNegativeDuration is returned when trying to record negative duration
	ErrNegativeDuration = errors.New("duration cannot be negative")
)

// Message represents a chat message
type Message struct {
	Content   string
	Timestamp time.Time
	Sender    string // "user", "ai", "admin"
	FileID    string
	FileURL   string
	Metadata  map[string]string
}

// Session represents an active user session
type Session struct {
	// Identity
	ID     string
	UserID string
	Name   string

	// Configuration
	ModelID string

	// Content
	Messages []*Message

	// Timing
	StartTime    time.Time
	LastActivity time.Time
	EndTime      *time.Time

	// State
	IsActive      bool
	HelpRequested bool

	// Admin Assistance
	AdminAssisted     bool
	AssistingAdminID  string
	AssistingAdminName string

	// Metrics
	TotalTokens   int
	ResponseTimes []time.Duration

	// Concurrency
	mu sync.RWMutex
}

// SessionManager manages active sessions
type SessionManager struct {
	sessions         map[string]*Session    // sessionID -> Session
	userSessions     map[string]string      // userID -> active sessionID
	mu               sync.RWMutex
	reconnectTimeout time.Duration
}

// NewSessionManager creates a new session manager
func NewSessionManager(reconnectTimeout time.Duration) *SessionManager {
	return &SessionManager{
		sessions:         make(map[string]*Session),
		userSessions:     make(map[string]string),
		reconnectTimeout: reconnectTimeout,
	}
}

// CreateSession creates a new session for a user
// Returns error if user ID is empty or user already has an active session
func (sm *SessionManager) CreateSession(userID string) (*Session, error) {
	if userID == "" {
		return nil, ErrInvalidUserID
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Check if user already has an active session
	if existingSessionID, exists := sm.userSessions[userID]; exists {
		if session, ok := sm.sessions[existingSessionID]; ok && session.IsActive {
			return nil, fmt.Errorf("%w: session %s", ErrActiveSessionExists, existingSessionID)
		}
	}

	// Create new session
	now := time.Now()
	session := &Session{
		ID:            uuid.New().String(),
		UserID:        userID,
		Name:          "",
		ModelID:       "",
		Messages:      []*Message{},
		StartTime:     now,
		LastActivity:  now,
		EndTime:       nil,
		IsActive:      true,
		HelpRequested: false,
		AdminAssisted: false,
		AssistingAdminID: "",
		AssistingAdminName: "",
		TotalTokens:   0,
		ResponseTimes: []time.Duration{},
	}

	// Store session and mapping
	sm.sessions[session.ID] = session
	sm.userSessions[userID] = session.ID

	return session, nil
}

// GetSession retrieves a session by ID
func (sm *SessionManager) GetSession(sessionID string) (*Session, error) {
	if sessionID == "" {
		return nil, ErrInvalidSessionID
	}

	sm.mu.RLock()
	defer sm.mu.RUnlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}

	return session, nil
}

// RestoreSession restores a session after reconnection
// Returns error if session not found, timed out, or doesn't belong to user
func (sm *SessionManager) RestoreSession(userID, sessionID string) (*Session, error) {
	if userID == "" {
		return nil, ErrInvalidUserID
	}
	if sessionID == "" {
		return nil, ErrInvalidSessionID
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}

	// Verify session belongs to user
	if session.UserID != userID {
		return nil, fmt.Errorf("%w: session %s belongs to %s, not %s", 
			ErrSessionOwnership, sessionID, session.UserID, userID)
	}

	// Check if session has timed out
	if session.EndTime != nil {
		timeSinceEnd := time.Since(*session.EndTime)
		if timeSinceEnd > sm.reconnectTimeout {
			return nil, fmt.Errorf("%w: session ended %v ago (timeout: %v)", 
				ErrSessionTimeout, timeSinceEnd, sm.reconnectTimeout)
		}
	}

	// Restore session
	session.IsActive = true
	session.LastActivity = time.Now()
	session.EndTime = nil

	// Restore user mapping
	sm.userSessions[userID] = sessionID

	return session, nil
}

// EndSession marks a session as ended
func (sm *SessionManager) EndSession(sessionID string) error {
	if sessionID == "" {
		return ErrInvalidSessionID
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}

	// Mark session as inactive
	now := time.Now()
	session.IsActive = false
	session.EndTime = &now

	// Remove user mapping
	delete(sm.userSessions, session.UserID)

	return nil
}

// SetSessionNameFromMessage sets the session name based on the first message
// This should be called when the first user message is added to a session
func (sm *SessionManager) SetSessionNameFromMessage(sessionID, message string) error {
	if sessionID == "" {
		return ErrInvalidSessionID
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}

	// Only set name if it's empty (first message)
	if session.Name == "" {
		session.Name = GenerateSessionName(message, 50)
	}

	return nil
}

// GenerateSessionName generates a descriptive session name from the first message
// It extracts the first sentence or line, truncates to maxLength, and returns a default
// name if the message is empty or whitespace-only.
func GenerateSessionName(firstMessage string, maxLength int) string {
	const defaultName = "New Chat"
	const ellipsis = "..."
	
	// Trim whitespace
	message := trimWhitespace(firstMessage)
	
	// Return default if empty
	if message == "" {
		return defaultName
	}
	
	// Extract first sentence or line
	name := extractFirstSentenceOrLine(message)
	
	// Truncate if necessary
	if len(name) > maxLength {
		// Make sure we have room for ellipsis
		if maxLength <= len(ellipsis) {
			return ellipsis
		}
		truncateAt := maxLength - len(ellipsis)
		
		// Try to truncate at word boundary
		name = truncateAtWordBoundary(name, truncateAt) + ellipsis
	}
	
	return name
}

// truncateAtWordBoundary truncates string at or before maxLen at a word boundary
func truncateAtWordBoundary(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	
	// Find the last space before maxLen
	truncated := s[:maxLen]
	lastSpace := -1
	for i := len(truncated) - 1; i >= 0; i-- {
		if truncated[i] == ' ' {
			lastSpace = i
			break
		}
	}
	
	// If we found a space, truncate there
	if lastSpace > 0 {
		return trimWhitespace(truncated[:lastSpace])
	}
	
	// No space found, just truncate at maxLen
	return truncated
}

// trimWhitespace removes leading and trailing whitespace including newlines and tabs
func trimWhitespace(s string) string {
	// Simple implementation without gohelper for now
	start := 0
	end := len(s)
	
	// Trim leading whitespace
	for start < end {
		c := s[start]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			break
		}
		start++
	}
	
	// Trim trailing whitespace
	for end > start {
		c := s[end-1]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			break
		}
		end--
	}
	
	return s[start:end]
}

// extractFirstSentenceOrLine extracts the first sentence (ending with . ? !) or first line
func extractFirstSentenceOrLine(s string) string {
	// Check for newline first
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' || s[i] == '\r' {
			return trimWhitespace(s[:i])
		}
	}
	
	// Check for sentence ending punctuation
	for i := 0; i < len(s); i++ {
		if s[i] == '.' || s[i] == '?' || s[i] == '!' {
			// Include the punctuation
			return trimWhitespace(s[:i+1])
		}
	}
	
	// No sentence ending or newline found, return the whole string
	return s
}

// UpdateTokenUsage adds tokens to the session's total token count
// Returns error if session not found or token count is negative
func (sm *SessionManager) UpdateTokenUsage(sessionID string, tokens int) error {
	if sessionID == "" {
		return ErrInvalidSessionID
	}
	
	if tokens < 0 {
		return ErrNegativeTokens
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	session.TotalTokens += tokens

	return nil
}

// RecordResponseTime records a response time for the session
// Returns error if session not found or duration is negative
func (sm *SessionManager) RecordResponseTime(sessionID string, duration time.Duration) error {
	if sessionID == "" {
		return ErrInvalidSessionID
	}
	
	if duration < 0 {
		return ErrNegativeDuration
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	session.ResponseTimes = append(session.ResponseTimes, duration)

	return nil
}

// GetMaxResponseTime returns the maximum response time for the session
// Returns 0 if no response times have been recorded
// Returns error if session not found
func (sm *SessionManager) GetMaxResponseTime(sessionID string) (time.Duration, error) {
	if sessionID == "" {
		return 0, ErrInvalidSessionID
	}

	sm.mu.RLock()
	defer sm.mu.RUnlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return 0, fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}

	session.mu.RLock()
	defer session.mu.RUnlock()

	if len(session.ResponseTimes) == 0 {
		return 0, nil
	}

	maxTime := session.ResponseTimes[0]
	for _, duration := range session.ResponseTimes[1:] {
		if duration > maxTime {
			maxTime = duration
		}
	}

	return maxTime, nil
}

// GetAverageResponseTime returns the average response time for the session
// Returns 0 if no response times have been recorded
// Returns error if session not found
func (sm *SessionManager) GetAverageResponseTime(sessionID string) (time.Duration, error) {
	if sessionID == "" {
		return 0, ErrInvalidSessionID
	}

	sm.mu.RLock()
	defer sm.mu.RUnlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return 0, fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}

	session.mu.RLock()
	defer session.mu.RUnlock()

	if len(session.ResponseTimes) == 0 {
		return 0, nil
	}

	var total time.Duration
	for _, duration := range session.ResponseTimes {
		total += duration
	}

	return total / time.Duration(len(session.ResponseTimes)), nil
}

// GetSessionDuration returns the duration of the session
// For active sessions, returns time from start to now
// For ended sessions, returns time from start to end
// Returns error if session not found
func (sm *SessionManager) GetSessionDuration(sessionID string) (time.Duration, error) {
	if sessionID == "" {
		return 0, ErrInvalidSessionID
	}

	sm.mu.RLock()
	defer sm.mu.RUnlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return 0, fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}

	session.mu.RLock()
	defer session.mu.RUnlock()

	if session.EndTime != nil {
		return session.EndTime.Sub(session.StartTime), nil
	}

	return time.Since(session.StartTime), nil
}
// SetModelID sets the model ID for the session
// Returns error if session not found or model ID is empty
func (sm *SessionManager) SetModelID(sessionID, modelID string) error {
	if sessionID == "" {
		return ErrInvalidSessionID
	}

	if modelID == "" {
		return errors.New("model ID cannot be empty")
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	session.ModelID = modelID

	return nil
}

// GetModelID returns the model ID for the session
// Returns empty string if no model is set
// Returns error if session not found
func (sm *SessionManager) GetModelID(sessionID string) (string, error) {
	if sessionID == "" {
		return "", ErrInvalidSessionID
	}

	sm.mu.RLock()
	defer sm.mu.RUnlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return "", fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}

	session.mu.RLock()
	defer session.mu.RUnlock()

	return session.ModelID, nil
}

