// LOCK ORDERING INVARIANT:
// SessionManager.mu must always be acquired BEFORE Session.mu.
// Never acquire SessionManager.mu while holding Session.mu — this will deadlock.
// Correct:  sm.mu.Lock() → session.mu.Lock()
// Wrong:    session.mu.Lock() → sm.mu.Lock()
package session

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/real-rm/gohelper"
	"github.com/real-rm/golog"
)

// MaxResponseTimes is the maximum number of response times to keep in memory
// This implements a rolling window to prevent unbounded memory growth
const MaxResponseTimes = 100

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

// Session represents an active user session.
// ID and UserID are immutable after construction -- safe to read without acquiring mu.
// All other fields require mu.RLock() for reads and mu.Lock() for writes.
type Session struct {
	// Identity (immutable after construction -- no lock required for reads)
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
	AdminAssisted      bool
	AssistingAdminID   string
	AssistingAdminName string

	// Metrics
	TotalTokens   int
	ResponseTimes []time.Duration

	// Concurrency
	mu sync.RWMutex
}

// SessionManager manages active sessions in memory.
// NOTE: In-memory sessions are NOT automatically synchronized across pods.
// On startup, call RehydrateFromStorage() to load active sessions from MongoDB.
// For horizontal scaling, configure K8s sticky sessions (sessionAffinity: ClientIP
// and ingress cookie affinity) to pin WebSocket connections to a single pod.
// True multi-pod session sharing requires a Redis-backed session store.
type SessionManager struct {
	sessions         map[string]*Session // sessionID -> Session
	userSessions     map[string]string   // userID -> active sessionID
	mu               sync.RWMutex
	reconnectTimeout time.Duration
	logger           *golog.Logger

	// Cleanup goroutine management
	cleanupInterval time.Duration
	sessionTTL      time.Duration
	stopCleanup     chan struct{}
	cleanupWg       sync.WaitGroup
	stopOnce        sync.Once
}

// StorageLoader provides the ability to load active sessions from persistent storage.
// This interface avoids a circular dependency between session and storage packages.
type StorageLoader interface {
	// LoadActiveSessions returns all sessions that have no end time (still active).
	LoadActiveSessions() ([]*Session, error)
}

// NewSessionManager creates a new session manager
func NewSessionManager(reconnectTimeout time.Duration, logger *golog.Logger) *SessionManager {
	sessionLogger := logger.WithGroup("session")
	return &SessionManager{
		sessions:         make(map[string]*Session),
		userSessions:     make(map[string]string),
		reconnectTimeout: reconnectTimeout,
		logger:           sessionLogger,
		cleanupInterval:  5 * time.Minute,  // Default: cleanup every 5 minutes
		sessionTTL:       15 * time.Minute, // Default: remove sessions 15 minutes after EndTime
		stopCleanup:      make(chan struct{}),
	}
}

// RehydrateFromStorage loads active sessions from persistent storage into the
// in-memory session map. This should be called once during startup, after
// creating the SessionManager, to restore sessions that survived a pod restart.
func (sm *SessionManager) RehydrateFromStorage(loader StorageLoader) error {
	sessions, err := loader.LoadActiveSessions()
	if err != nil {
		return fmt.Errorf("failed to load active sessions: %w", err)
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	loaded := 0
	for _, sess := range sessions {
		if sess == nil || sess.ID == "" || sess.UserID == "" {
			continue
		}
		sm.sessions[sess.ID] = sess
		sm.userSessions[sess.UserID] = sess.ID
		loaded++
	}

	sm.logger.Info("Rehydrated sessions from storage", "loaded", loaded, "total_found", len(sessions))
	return nil
}

// CreateSession creates a new session for a user.
// Returns error if user ID is empty or user already has an active session.
// NOTE: The returned *Session pointer is shared. Callers must use Session.mu
// for any field mutations to avoid data races with concurrent goroutines.
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

	// Generate session ID using gohelper
	sessionID, err := gohelper.GenUUID(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate session ID: %w", err)
	}

	session := &Session{
		ID:                 sessionID,
		UserID:             userID,
		Name:               "",
		ModelID:            "",
		Messages:           []*Message{},
		StartTime:          now,
		LastActivity:       now,
		EndTime:            nil,
		IsActive:           true,
		HelpRequested:      false,
		AdminAssisted:      false,
		AssistingAdminID:   "",
		AssistingAdminName: "",
		TotalTokens:        0,
		ResponseTimes:      []time.Duration{},
	}

	// Store session and mapping
	sm.sessions[session.ID] = session
	sm.userSessions[userID] = session.ID

	sm.logger.Info("Session created", "session_id", session.ID, "user_id", userID)
	return session, nil
}

// GetSession retrieves a session by ID.
// NOTE: The returned *Session pointer is shared. Callers must use Session.mu
// for any field mutations to avoid data races with concurrent goroutines.
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

// RestoreSession restores a session after reconnection.
// Returns error if session not found, timed out, or doesn't belong to user.
// NOTE: The returned *Session pointer is shared. Callers must use Session.mu
// for any field mutations to avoid data races with concurrent goroutines.
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

	// Restore session — acquire session.mu per lock ordering (sm.mu → session.mu)
	session.mu.Lock()
	session.IsActive = true
	session.LastActivity = time.Now()
	session.EndTime = nil
	session.mu.Unlock()

	// Restore user mapping
	sm.userSessions[userID] = sessionID

	sm.logger.Info("Session restored", "session_id", sessionID, "user_id", userID)
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

	// Mark session as inactive — acquire session.mu per lock ordering (sm.mu → session.mu)
	session.mu.Lock()
	now := time.Now()
	session.IsActive = false
	session.EndTime = &now
	session.mu.Unlock()

	// Remove user mapping
	delete(sm.userSessions, session.UserID)

	sm.logger.Info("Session ended", "session_id", sessionID, "user_id", session.UserID, "duration", time.Since(session.StartTime))
	return nil
}

// StartCleanup starts the background cleanup goroutine
// This should be called after creating the SessionManager
func (sm *SessionManager) StartCleanup() {
	sm.cleanupWg.Add(1)
	go func() {
		defer sm.cleanupWg.Done()
		ticker := time.NewTicker(sm.cleanupInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				sm.cleanupExpiredSessions()
			case <-sm.stopCleanup:
				return
			}
		}
	}()
}

// cleanupExpiredSessions removes inactive sessions that have exceeded the TTL
// This method should only be called by the cleanup goroutine
func (sm *SessionManager) cleanupExpiredSessions() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now()
	removed := 0

	for sessionID, sess := range sm.sessions {
		sess.mu.RLock()
		isActive := sess.IsActive
		endTime := sess.EndTime
		sess.mu.RUnlock()

		if !isActive && endTime != nil {
			if now.Sub(*endTime) > sm.sessionTTL {
				delete(sm.sessions, sessionID)
				removed++
			}
		}
	}

	if removed > 0 {
		sm.logger.Info("Cleaned up expired sessions", "count", removed)
	}
}

// StopCleanup stops the background cleanup goroutine.
// Safe to call concurrently and multiple times.
func (sm *SessionManager) StopCleanup() {
	sm.stopOnce.Do(func() {
		close(sm.stopCleanup)
	})
	sm.cleanupWg.Wait()
}

// GetMemoryStats returns the current memory statistics for sessions
// Returns active, inactive, and total session counts
func (sm *SessionManager) GetMemoryStats() (active, inactive, total int) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	for _, sess := range sm.sessions {
		total++
		sess.mu.RLock()
		isActive := sess.IsActive
		sess.mu.RUnlock()
		if isActive {
			active++
		} else {
			inactive++
		}
	}
	return
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
	session.mu.Lock()
	if session.Name == "" {
		session.Name = GenerateSessionName(message, 50)
	}
	session.mu.Unlock()

	return nil
}

// GenerateSessionName generates a descriptive session name from the first message
// It extracts the first sentence or line, truncates to maxLength, and returns a default
// name if the message is empty or whitespace-only.
func GenerateSessionName(firstMessage string, maxLength int) string {
	const defaultName = "New Chat"
	const ellipsis = "..."

	// Trim whitespace
	message := strings.TrimSpace(firstMessage)

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
		return strings.TrimSpace(truncated[:lastSpace])
	}

	// No space found, just truncate at maxLen
	return truncated
}

// extractFirstSentenceOrLine extracts the first sentence (ending with . ? !) or first line
func extractFirstSentenceOrLine(s string) string {
	// Check for newline first
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' || s[i] == '\r' {
			return strings.TrimSpace(s[:i])
		}
	}

	// Check for sentence ending punctuation
	for i := 0; i < len(s); i++ {
		if s[i] == '.' || s[i] == '?' || s[i] == '!' {
			// Include the punctuation
			return strings.TrimSpace(s[:i+1])
		}
	}

	// No sentence ending or newline found, return the whole string
	return s
}

// AddMessage adds a message to the session
// Returns error if session not found or message is nil
func (sm *SessionManager) AddMessage(sessionID string, msg *Message) error {
	if sessionID == "" {
		return ErrInvalidSessionID
	}

	if msg == nil {
		return errors.New("message cannot be nil")
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}

	// Add message to session — acquire session.mu per lock ordering (sm.mu → session.mu)
	session.mu.Lock()
	session.Messages = append(session.Messages, msg)
	session.LastActivity = time.Now()
	session.mu.Unlock()

	sm.logger.Debug("Message added to session",
		"session_id", sessionID,
		"message_count", len(session.Messages))

	return nil
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
// Implements a rolling window to prevent unbounded memory growth
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

	// Implement rolling window with fixed maximum size
	if len(session.ResponseTimes) >= MaxResponseTimes {
		// Remove oldest entry (shift left)
		copy(session.ResponseTimes, session.ResponseTimes[1:])
		session.ResponseTimes[MaxResponseTimes-1] = duration
	} else {
		session.ResponseTimes = append(session.ResponseTimes, duration)
	}

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

// MarkHelpRequested marks a session as requiring assistance
// Returns error if session not found
func (sm *SessionManager) MarkHelpRequested(sessionID string) error {
	if sessionID == "" {
		return ErrInvalidSessionID
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	session.HelpRequested = true
	session.LastActivity = time.Now()

	sm.logger.Info("Help requested for session", "session_id", sessionID, "user_id", session.UserID)
	return nil
}

// IsHelpRequested returns whether a session has requested help
// Returns error if session not found
func (sm *SessionManager) IsHelpRequested(sessionID string) (bool, error) {
	if sessionID == "" {
		return false, ErrInvalidSessionID
	}

	sm.mu.RLock()
	defer sm.mu.RUnlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return false, fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}

	session.mu.RLock()
	defer session.mu.RUnlock()

	return session.HelpRequested, nil
}

// MarkAdminAssisted marks a session as having been assisted by an admin
// Returns error if session not found or admin ID/name is empty
func (sm *SessionManager) MarkAdminAssisted(sessionID, adminID, adminName string) error {
	if sessionID == "" {
		return ErrInvalidSessionID
	}
	if adminID == "" {
		return errors.New("admin ID cannot be empty")
	}
	if adminName == "" {
		return errors.New("admin name cannot be empty")
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	// Atomic check-and-set: reject if a different admin is already assisting
	if session.AssistingAdminID != "" && session.AssistingAdminID != adminID {
		return fmt.Errorf("session already assisted by admin %s (%s)", session.AssistingAdminName, session.AssistingAdminID)
	}

	session.AdminAssisted = true
	session.AssistingAdminID = adminID
	session.AssistingAdminName = adminName
	session.LastActivity = time.Now()

	sm.logger.Info("Admin joined session",
		"session_id", sessionID,
		"admin_id", adminID,
		"admin_name", adminName)
	return nil
}

// ClearAdminAssistance clears admin assistance from a session
// Returns error if session not found
func (sm *SessionManager) ClearAdminAssistance(sessionID string) error {
	if sessionID == "" {
		return ErrInvalidSessionID
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	adminID := session.AssistingAdminID
	session.AssistingAdminID = ""
	session.AssistingAdminName = ""
	session.LastActivity = time.Now()

	sm.logger.Info("Admin left session",
		"session_id", sessionID,
		"admin_id", adminID)
	return nil
}

// GetAssistingAdmin returns the admin ID and name assisting a session
// Returns empty strings if no admin is assisting
// Returns error if session not found
func (sm *SessionManager) GetAssistingAdmin(sessionID string) (string, string, error) {
	if sessionID == "" {
		return "", "", ErrInvalidSessionID
	}

	sm.mu.RLock()
	defer sm.mu.RUnlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return "", "", fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}

	session.mu.RLock()
	defer session.mu.RUnlock()

	return session.AssistingAdminID, session.AssistingAdminName, nil
}

// GetModelID returns the session's model ID in a thread-safe manner.
func (s *Session) GetModelID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ModelID
}

// GetAssistingAdminID returns the assisting admin's ID in a thread-safe manner.
func (s *Session) GetAssistingAdminID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.AssistingAdminID
}

// GetAssistingAdminName returns the assisting admin's name in a thread-safe manner.
func (s *Session) GetAssistingAdminName() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.AssistingAdminName
}

// GetAdminAssistance returns the assisting admin ID and name in a thread-safe manner.
func (s *Session) GetAdminAssistance() (adminID, adminName string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.AssistingAdminID, s.AssistingAdminName
}

// RLock acquires a read lock on the session.
// WARNING: Do not acquire SessionManager.mu while holding this lock.
// Lock ordering: SessionManager.mu → Session.mu
func (s *Session) RLock() {
	s.mu.RLock()
}

// RUnlock releases a read lock on the session
func (s *Session) RUnlock() {
	s.mu.RUnlock()
}

// Lock acquires a write lock on the session.
// WARNING: Do not acquire SessionManager.mu while holding this lock.
// Lock ordering: SessionManager.mu → Session.mu
func (s *Session) Lock() {
	s.mu.Lock()
}

// Unlock releases a write lock on the session
func (s *Session) Unlock() {
	s.mu.Unlock()
}
