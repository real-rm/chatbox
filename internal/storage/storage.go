package storage

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/real-rm/chatbox/internal/constants"
	"github.com/real-rm/chatbox/internal/metrics"
	"github.com/real-rm/chatbox/internal/session"
	"github.com/real-rm/chatbox/internal/util"
	"github.com/real-rm/golog"
	"github.com/real-rm/gomongo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	// ErrInvalidSession is returned when session is nil
	ErrInvalidSession = errors.New("session cannot be nil")
	// ErrInvalidSessionID is returned when session ID is empty
	ErrInvalidSessionID = errors.New("session ID cannot be empty")
	// ErrSessionNotFound is returned when session is not found in database
	ErrSessionNotFound = errors.New("session not found in database")
)

// retryConfig holds configuration for MongoDB retry logic
type retryConfig struct {
	maxAttempts  int
	initialDelay time.Duration
	maxDelay     time.Duration
	multiplier   float64
}

// defaultRetryConfig provides default retry configuration
var defaultRetryConfig = retryConfig{
	maxAttempts:  constants.MaxRetryAttempts,
	initialDelay: constants.InitialRetryDelay,
	maxDelay:     constants.MaxRetryDelay,
	multiplier:   constants.RetryMultiplier,
}

// StorageService manages conversation persistence in MongoDB using gomongo
type StorageService struct {
	mongo         *gomongo.Mongo
	collection    *gomongo.MongoCollection
	logger        *golog.Logger
	encryptionKey []byte // Key for encrypting sensitive fields
}

// SessionDocument represents a session stored in MongoDB
type SessionDocument struct {
	ID                 string            `bson:"_id"`
	UserID             string            `bson:"uid"`
	Name               string            `bson:"nm"`
	ModelID            string            `bson:"modelId"`
	Messages           []MessageDocument `bson:"msgs"`
	StartTime          time.Time         `bson:"ts"`
	EndTime            *time.Time        `bson:"endTs,omitempty"`
	Duration           int64             `bson:"dur"` // seconds
	AdminAssisted      bool              `bson:"adminAssisted"`
	AssistingAdminID   string            `bson:"assistingAdminId,omitempty"`
	AssistingAdminName string            `bson:"assistingAdminName,omitempty"`
	HelpRequested      bool              `bson:"helpRequested"`
	TotalTokens        int               `bson:"totalTokens"`
	MaxResponseTime    int64             `bson:"maxRespTime"`   // milliseconds
	AvgResponseTime    int64             `bson:"avgRespTime"`   // milliseconds
	CreatedAt          time.Time         `bson:"_ts,omitempty"` // gomongo automatic timestamp
	ModifiedAt         time.Time         `bson:"_mt,omitempty"` // gomongo automatic timestamp
}

// MessageDocument represents a message stored in MongoDB
type MessageDocument struct {
	Content   string            `bson:"content"`
	Timestamp time.Time         `bson:"ts"`
	Sender    string            `bson:"sender"` // "user", "ai", "admin"
	FileID    string            `bson:"fileId,omitempty"`
	FileURL   string            `bson:"fileUrl,omitempty"`
	Metadata  map[string]string `bson:"meta,omitempty"`
}

// SessionMetadata represents summary information about a session
type SessionMetadata struct {
	ID              string
	UserID          string // User ID for admin views
	Name            string
	LastMessageTime time.Time
	MessageCount    int
	AdminAssisted   bool
	StartTime       time.Time
	EndTime         *time.Time
	TotalTokens     int
	MaxResponseTime int64 // milliseconds
	AvgResponseTime int64 // milliseconds
}

// SessionListOptions defines filtering, sorting, and pagination options for listing sessions
type SessionListOptions struct {
	// Pagination
	Limit  int // Maximum number of results to return (default: 100, max: 1000)
	Offset int // Number of results to skip for pagination

	// Filtering
	UserID        string     // Filter by specific user ID
	StartTimeFrom *time.Time // Filter sessions starting after this time
	StartTimeTo   *time.Time // Filter sessions starting before this time
	AdminAssisted *bool      // Filter by admin assistance status (nil = all, true = assisted only, false = not assisted)
	Active        *bool      // Filter by active status (nil = all, true = active only, false = ended only)

	// Sorting
	SortBy    string // Field to sort by: "ts", "endTs", "message_count", "totalTokens", "uid"
	SortOrder string // Sort order: "asc" or "desc" (default: "desc")
}

// Metrics represents aggregated session metrics for admin monitoring
type Metrics struct {
	TotalSessions      int
	ActiveSessions     int
	AvgConcurrent      float64
	MaxConcurrent      int
	TotalTokens        int
	AvgResponseTime    int64 // milliseconds
	MaxResponseTime    int64 // milliseconds
	AdminAssistedCount int
}

// NewStorageService creates a new storage service using gomongo
// mongo: gomongo.Mongo instance (from gomongo.InitMongoDB)
// dbName: database name
// collName: collection name
// logger: golog.Logger instance for logging
// encryptionKey: should be 32 bytes for AES-256 encryption
func NewStorageService(mongo *gomongo.Mongo, dbName, collName string, logger *golog.Logger, encryptionKey []byte) *StorageService {
	collection := mongo.Coll(dbName, collName)

	return &StorageService{
		mongo:         mongo,
		collection:    collection,
		logger:        logger,
		encryptionKey: encryptionKey,
	}
}

// isRetryableError checks if an error is retryable (transient)
// Returns true for network errors and transient MongoDB errors
func isRetryableError(err error) bool {
	// No else needed: early return pattern (guard clause)
	if err == nil {
		return false
	}

	errStr := err.Error()

	// Network errors
	if containsAny(errStr, []string{
		"connection refused",
		"connection reset",
		"timeout",
		"temporary failure",
		"i/o timeout",
		"EOF",
	}) {
		return true
	}

	// MongoDB specific transient errors
	if containsAny(errStr, []string{
		"server selection timeout",
		"no reachable servers",
		"connection pool",
		"socket",
	}) {
		return true
	}

	return false
}

// containsAny checks if a string contains any of the given substrings
func containsAny(s string, substrings []string) bool {
	for _, substr := range substrings {
		if len(s) >= len(substr) {
			for i := 0; i <= len(s)-len(substr); i++ {
				match := true
				for j := 0; j < len(substr); j++ {
					if s[i+j] != substr[j] {
						match = false
						break
					}
				}
				if match {
					return true
				}
			}
		}
	}
	return false
}

// EnsureIndexes creates the necessary indexes for the sessions collection
// This should be called during application initialization to ensure optimal query performance
func (s *StorageService) EnsureIndexes(ctx context.Context) error {
	// Create index for user_id (uid) - used for user-specific session queries
	userIDIndex := mongo.IndexModel{
		Keys:    bson.D{{Key: constants.MongoFieldUserID, Value: 1}},
		Options: options.Index().SetName(constants.IndexUserID),
	}

	// Create index for start_time (ts) - used for time-based queries and sorting
	startTimeIndex := mongo.IndexModel{
		Keys:    bson.D{{Key: constants.MongoFieldTimestamp, Value: -1}}, // Descending for most recent first
		Options: options.Index().SetName(constants.IndexStartTime),
	}

	// Create index for admin_assisted - used for filtering admin-assisted sessions
	adminAssistedIndex := mongo.IndexModel{
		Keys:    bson.D{{Key: constants.MongoFieldAdminAssisted, Value: 1}},
		Options: options.Index().SetName(constants.IndexAdminAssisted),
	}

	// Create compound index for common query patterns (user_id + start_time)
	compoundIndex := mongo.IndexModel{
		Keys: bson.D{
			{Key: constants.MongoFieldUserID, Value: 1},
			{Key: constants.MongoFieldTimestamp, Value: -1},
		},
		Options: options.Index().SetName(constants.IndexUserStartTime),
	}

	// Create all indexes
	indexes := []mongo.IndexModel{
		userIDIndex,
		startTimeIndex,
		adminAssistedIndex,
		compoundIndex,
	}

	_, err := s.collection.CreateIndexes(ctx, indexes)
	// No else needed: early return pattern (guard clause)
	if err != nil {
		return fmt.Errorf("failed to create indexes: %w", err)
	}

	s.logger.Info("MongoDB indexes created successfully",
		"indexes", []string{constants.IndexUserID, constants.IndexStartTime, constants.IndexAdminAssisted, constants.IndexUserStartTime},
	)

	return nil
}

// CreateSession creates a new session document in MongoDB
func (s *StorageService) CreateSession(sess *session.Session) error {
	// No else needed: early return pattern (guard clause)
	if sess == nil {
		return ErrInvalidSession
	}

	// No else needed: early return pattern (guard clause)
	if sess.ID == "" {
		return ErrInvalidSessionID
	}

	ctx, cancel := util.NewTimeoutContext(constants.DefaultContextTimeout)
	defer cancel()

	// Convert session to document
	doc := s.sessionToDocument(sess)

	// Insert document with retry logic for transient errors
	err := s.retryOperation(ctx, "CreateSession", func() error {
		_, err := s.collection.InsertOne(ctx, doc)
		return err
	})

	// No else needed: early return pattern (guard clause)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	// Increment session metrics
	metrics.SessionsCreated.Inc()
	metrics.ActiveSessions.Inc()

	return nil
}

// UpdateSession updates an existing session document in MongoDB
func (s *StorageService) UpdateSession(sess *session.Session) error {
	// No else needed: early return pattern (guard clause)
	if sess == nil {
		return ErrInvalidSession
	}

	// No else needed: early return pattern (guard clause)
	if sess.ID == "" {
		return ErrInvalidSessionID
	}

	ctx, cancel := util.NewTimeoutContext(constants.DefaultContextTimeout)
	defer cancel()

	// Convert session to document
	doc := s.sessionToDocument(sess)

	// Update document with retry logic for transient errors
	filter := bson.M{constants.MongoFieldID: sess.ID}
	update := bson.M{"$set": doc}

	var result *mongo.UpdateResult
	err := s.retryOperation(ctx, "UpdateSession", func() error {
		var err error
		result, err = s.collection.UpdateOne(ctx, filter, update)
		return err
	})

	// No else needed: early return pattern (guard clause)
	if err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	// No else needed: early return pattern (guard clause)
	if result.MatchedCount == 0 {
		return ErrSessionNotFound
	}

	return nil
}

// GetSession retrieves a session from MongoDB by ID
func (s *StorageService) GetSession(sessionID string) (*session.Session, error) {
	// No else needed: early return pattern (guard clause)
	if sessionID == "" {
		return nil, ErrInvalidSessionID
	}

	ctx, cancel := util.NewTimeoutContext(constants.DefaultContextTimeout)
	defer cancel()

	// Find document with retry logic for transient errors
	filter := bson.M{constants.MongoFieldID: sessionID}
	var doc SessionDocument

	err := s.retryOperation(ctx, "GetSession", func() error {
		result := s.collection.FindOne(ctx, filter)
		return result.Decode(&doc)
	})

	// No else needed: early return pattern (guard clause)
	// CRITICAL FIX C4: Use errors.Is for proper error comparison
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	// Convert document to session
	sess := s.documentToSession(&doc)

	return sess, nil
}

// sessionToDocument converts a Session to a SessionDocument
// This method acquires a read lock on the session to ensure thread-safe access
func (s *StorageService) sessionToDocument(sess *session.Session) *SessionDocument {
	// Acquire read lock to prevent data races during serialization
	sess.RLock()
	defer sess.RUnlock()

	// Convert messages
	messages := make([]MessageDocument, len(sess.Messages))
	for i, msg := range sess.Messages {
		messages[i] = MessageDocument{
			Content:   msg.Content,
			Timestamp: msg.Timestamp,
			Sender:    msg.Sender,
			FileID:    msg.FileID,
			FileURL:   msg.FileURL,
			Metadata:  msg.Metadata,
		}
	}

	// Calculate duration
	var duration int64
	if sess.EndTime != nil {
		duration = int64(sess.EndTime.Sub(sess.StartTime).Seconds())
	} else {
		duration = int64(time.Since(sess.StartTime).Seconds())
	}

	// Calculate max and average response times
	var maxResponseTime, avgResponseTime int64
	// No else needed: optional operation (only calculate if response times exist)
	if len(sess.ResponseTimes) > 0 {
		var total time.Duration
		maxDuration := sess.ResponseTimes[0]

		for _, rt := range sess.ResponseTimes {
			total += rt
			// No else needed: optional operation (only update max if larger)
			if rt > maxDuration {
				maxDuration = rt
			}
		}

		maxResponseTime = maxDuration.Milliseconds()
		avgResponseTime = (total / time.Duration(len(sess.ResponseTimes))).Milliseconds()
	}

	return &SessionDocument{
		ID:                 sess.ID,
		UserID:             sess.UserID,
		Name:               sess.Name,
		ModelID:            sess.ModelID,
		Messages:           messages,
		StartTime:          sess.StartTime,
		EndTime:            sess.EndTime,
		Duration:           duration,
		AdminAssisted:      sess.AdminAssisted,
		AssistingAdminID:   sess.AssistingAdminID,
		AssistingAdminName: sess.AssistingAdminName,
		HelpRequested:      sess.HelpRequested,
		TotalTokens:        sess.TotalTokens,
		MaxResponseTime:    maxResponseTime,
		AvgResponseTime:    avgResponseTime,
	}
}

// documentToSession converts a SessionDocument to a Session
func (s *StorageService) documentToSession(doc *SessionDocument) *session.Session {
	// Convert messages and decrypt content
	messages := make([]*session.Message, len(doc.Messages))
	for i, msg := range doc.Messages {
		content := msg.Content
		// Decrypt content if encryption key is provided
		// No else needed: optional operation (only decrypt if key is available)
		if len(s.encryptionKey) > 0 {
			decrypted, err := s.decrypt(msg.Content)
			// No else needed: optional operation (fallback to original on error)
			if err == nil {
				content = decrypted
			}
			// If decryption fails, use original content (might be unencrypted)
		}

		messages[i] = &session.Message{
			Content:   content,
			Timestamp: msg.Timestamp,
			Sender:    msg.Sender,
			FileID:    msg.FileID,
			FileURL:   msg.FileURL,
			Metadata:  msg.Metadata,
		}
	}

	// Reconstruct response times from max and avg
	// Note: We can't perfectly reconstruct the original response times,
	// but we can create a reasonable approximation
	var responseTimes []time.Duration
	// No else needed: optional operation (only reconstruct if data exists)
	if doc.MaxResponseTime > 0 && doc.AvgResponseTime > 0 {
		// Create a single entry with the average (simplified)
		responseTimes = []time.Duration{
			time.Duration(doc.AvgResponseTime) * time.Millisecond,
		}
	}

	// Determine if session is active
	isActive := doc.EndTime == nil

	return &session.Session{
		ID:                 doc.ID,
		UserID:             doc.UserID,
		Name:               doc.Name,
		ModelID:            doc.ModelID,
		Messages:           messages,
		StartTime:          doc.StartTime,
		LastActivity:       doc.StartTime, // Set to start time, will be updated
		EndTime:            doc.EndTime,
		IsActive:           isActive,
		HelpRequested:      doc.HelpRequested,
		AdminAssisted:      doc.AdminAssisted,
		AssistingAdminID:   doc.AssistingAdminID,
		AssistingAdminName: doc.AssistingAdminName,
		TotalTokens:        doc.TotalTokens,
		ResponseTimes:      responseTimes,
	}
}

// AddMessage adds a message to an existing session and persists it immediately
func (s *StorageService) AddMessage(sessionID string, msg *session.Message) error {
	// No else needed: early return pattern (guard clause)
	if sessionID == "" {
		return ErrInvalidSessionID
	}

	// No else needed: early return pattern (guard clause)
	if msg == nil {
		return errors.New("message cannot be nil")
	}

	ctx, cancel := util.NewTimeoutContext(constants.MessageAddTimeout)
	defer cancel()

	// Convert message to document
	msgDoc := MessageDocument{
		Content:   msg.Content,
		Timestamp: msg.Timestamp,
		Sender:    msg.Sender,
		FileID:    msg.FileID,
		FileURL:   msg.FileURL,
		Metadata:  msg.Metadata,
	}

	// Encrypt sensitive content if encryption key is provided
	// No else needed: optional operation (only encrypt if key is available)
	if len(s.encryptionKey) > 0 {
		encrypted, err := s.encrypt(msgDoc.Content)
		// No else needed: early return pattern (guard clause)
		if err != nil {
			return fmt.Errorf("failed to encrypt message content: %w", err)
		}
		msgDoc.Content = encrypted
	}

	// Push message to messages array using gomongo (automatically updates _mt)
	filter := bson.M{constants.MongoFieldID: sessionID}
	update := bson.M{
		"$push": bson.M{constants.MongoFieldMessages: msgDoc},
		"$set":  bson.M{"lastActivity": time.Now()},
	}

	result, err := s.collection.UpdateOne(ctx, filter, update)
	// No else needed: early return pattern (guard clause)
	if err != nil {
		return fmt.Errorf("failed to add message: %w", err)
	}

	// No else needed: early return pattern (guard clause)
	if result.MatchedCount == 0 {
		return ErrSessionNotFound
	}

	return nil
}

// EndSession updates the session with end timestamp and duration
func (s *StorageService) EndSession(sessionID string, endTime time.Time) error {
	// No else needed: early return pattern (guard clause)
	if sessionID == "" {
		return ErrInvalidSessionID
	}

	ctx, cancel := util.NewTimeoutContext(constants.SessionEndTimeout)
	defer cancel()

	// Get the session to calculate duration
	var doc SessionDocument
	err := s.collection.FindOne(ctx, bson.M{constants.MongoFieldID: sessionID}).Decode(&doc)
	// No else needed: early return pattern (guard clause)
	// CRITICAL FIX C4: Use errors.Is for proper error comparison
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return ErrSessionNotFound
		}
		return fmt.Errorf("failed to get session: %w", err)
	}

	// Calculate duration
	duration := int64(endTime.Sub(doc.StartTime).Seconds())

	// Update session with end time and duration using gomongo (automatically updates _mt)
	filter := bson.M{constants.MongoFieldID: sessionID}
	update := bson.M{
		"$set": bson.M{
			constants.MongoFieldEndTime:  endTime,
			constants.MongoFieldDuration: duration,
		},
	}

	result, err := s.collection.UpdateOne(ctx, filter, update)
	// No else needed: early return pattern (guard clause)
	if err != nil {
		return fmt.Errorf("failed to end session: %w", err)
	}

	// No else needed: early return pattern (guard clause)
	if result.MatchedCount == 0 {
		return ErrSessionNotFound
	}

	// Decrement active sessions metric
	metrics.SessionsEnded.Inc()
	metrics.ActiveSessions.Dec()

	return nil
}

// encrypt encrypts data using AES-256-GCM
func (s *StorageService) encrypt(plaintext string) (string, error) {
	// Validate key size (AES supports 16, 24, or 32 bytes)
	keyLen := len(s.encryptionKey)
	if keyLen != 0 && keyLen != 16 && keyLen != 24 && keyLen != 32 {
		return "", fmt.Errorf("invalid encryption key size: %d bytes (must be 16, 24, or 32)", keyLen)
	}
	
	// If no key is set (0 bytes), treat as "no encryption"
	if keyLen == 0 {
		return plaintext, nil
	}

	block, err := aes.NewCipher(s.encryptionKey)
	// No else needed: early return pattern (guard clause)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	// No else needed: early return pattern (guard clause)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	// Create nonce
	nonce := make([]byte, gcm.NonceSize())
	// No else needed: early return pattern (guard clause)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt and prepend nonce
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)

	// Encode to base64 for storage
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decrypt decrypts data using AES-256-GCM
func (s *StorageService) decrypt(ciphertext string) (string, error) {
	// No else needed: early return pattern (guard clause)
	if len(s.encryptionKey) == 0 {
		return ciphertext, nil
	}

	// Decode from base64
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	// No else needed: early return pattern (guard clause)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	block, err := aes.NewCipher(s.encryptionKey)
	// No else needed: early return pattern (guard clause)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	// No else needed: early return pattern (guard clause)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	// No else needed: early return pattern (guard clause)
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	// Extract nonce and ciphertext
	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]

	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	// No else needed: early return pattern (guard clause)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}

	return string(plaintext), nil
}

// ListUserSessions retrieves all sessions for a user ordered by last activity (most recent first)
// The limit parameter controls the maximum number of sessions to return (0 = no limit)
func (s *StorageService) ListUserSessions(userID string, limit int) ([]*SessionMetadata, error) {
	// No else needed: early return pattern (guard clause)
	if userID == "" {
		return nil, errors.New("user ID cannot be empty")
	}

	ctx, cancel := util.NewTimeoutContext(constants.DefaultContextTimeout)
	defer cancel()

	// Build query filter
	filter := bson.M{constants.MongoFieldUserID: userID}

	// Build find options with sorting by ts (descending)
	findOptions := options.Find()
	findOptions.SetSort(bson.D{{Key: constants.MongoFieldTimestamp, Value: -1}})

	if limit > 0 {
		findOptions.SetLimit(int64(limit))
	}

	// Execute query using gomongo
	cursor, err := s.collection.Find(ctx, filter, findOptions)
	// No else needed: early return pattern (guard clause)
	if err != nil {
		return nil, fmt.Errorf("failed to list user sessions: %w", err)
	}
	defer cursor.Close(ctx)

	// Decode results
	var sessions []*SessionMetadata
	for cursor.Next(ctx) {
		var doc SessionDocument
		if err := cursor.Decode(&doc); err != nil {
			return nil, fmt.Errorf("failed to decode session document: %w", err)
		}

		// Determine last message time
		lastMessageTime := doc.StartTime
		if len(doc.Messages) > 0 {
			lastMessageTime = doc.Messages[len(doc.Messages)-1].Timestamp
		}

		metadata := &SessionMetadata{
			ID:              doc.ID,
			UserID:          doc.UserID,
			Name:            doc.Name,
			LastMessageTime: lastMessageTime,
			MessageCount:    len(doc.Messages),
			AdminAssisted:   doc.AdminAssisted,
			StartTime:       doc.StartTime,
			EndTime:         doc.EndTime,
			TotalTokens:     doc.TotalTokens,
			MaxResponseTime: doc.MaxResponseTime,
			AvgResponseTime: doc.AvgResponseTime,
		}

		sessions = append(sessions, metadata)
	}

	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("cursor error: %w", err)
	}

	return sessions, nil
}

// ListAllSessions retrieves all sessions across all users ordered by start time (most recent first)
// The limit parameter controls the maximum number of sessions to return (0 = no limit)
// This is primarily used by admin endpoints to view all sessions in the system
func (s *StorageService) ListAllSessions(limit int) ([]*SessionMetadata, error) {
	ctx, cancel := util.NewTimeoutContext(constants.DefaultContextTimeout)
	defer cancel()

	// Build find options with sorting by ts (descending)
	findOptions := options.Find()
	findOptions.SetSort(bson.D{{Key: constants.MongoFieldTimestamp, Value: -1}})

	// No else needed: optional operation (only set limit if specified)
	if limit > 0 {
		findOptions.SetLimit(int64(limit))
	}

	// Execute query using gomongo (no filter = all documents)
	cursor, err := s.collection.Find(ctx, bson.M{}, findOptions)
	// No else needed: early return pattern (guard clause)
	if err != nil {
		return nil, fmt.Errorf("failed to list all sessions: %w", err)
	}
	defer cursor.Close(ctx)

	// Decode results
	var sessions []*SessionMetadata
	for cursor.Next(ctx) {
		var doc SessionDocument
		// No else needed: early return pattern (guard clause)
		if err := cursor.Decode(&doc); err != nil {
			return nil, fmt.Errorf("failed to decode session document: %w", err)
		}

		// Determine last message time
		lastMessageTime := doc.StartTime
		// No else needed: optional operation (only update if messages exist)
		if len(doc.Messages) > 0 {
			lastMessageTime = doc.Messages[len(doc.Messages)-1].Timestamp
		}

		metadata := &SessionMetadata{
			ID:              doc.ID,
			UserID:          doc.UserID, // Include user ID for admin view
			Name:            doc.Name,
			LastMessageTime: lastMessageTime,
			MessageCount:    len(doc.Messages),
			AdminAssisted:   doc.AdminAssisted,
			StartTime:       doc.StartTime,
			EndTime:         doc.EndTime,
			TotalTokens:     doc.TotalTokens,
			MaxResponseTime: doc.MaxResponseTime,
			AvgResponseTime: doc.AvgResponseTime,
		}

		sessions = append(sessions, metadata)
	}

	// No else needed: early return pattern (guard clause)
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("cursor error: %w", err)
	}

	return sessions, nil
}

// ListAllSessionsWithOptions lists all sessions with filtering, sorting, and pagination
// This method is designed for admin dashboards to efficiently query large session datasets
func (s *StorageService) ListAllSessionsWithOptions(opts *SessionListOptions) ([]*SessionMetadata, error) {
	ctx, cancel := util.NewTimeoutContext(constants.MetricsTimeout)
	defer cancel()

	// Set defaults
	if opts == nil {
		opts = &SessionListOptions{}
	}
	if opts.Limit <= 0 {
		opts.Limit = constants.DefaultSessionLimit
	}
	if opts.Limit > constants.MaxSessionLimit {
		opts.Limit = constants.MaxSessionLimit // Cap at max for performance
	}
	if opts.SortBy == "" {
		opts.SortBy = constants.SortByTimestamp
	}
	if opts.SortOrder == "" {
		opts.SortOrder = constants.SortOrderDesc
	}

	// Build filter
	filter := bson.M{}

	// No else needed: optional operation (only add filter if specified)
	if opts.UserID != "" {
		filter[constants.MongoFieldUserID] = opts.UserID
	}

	// No else needed: optional operation (only add filter if specified)
	if opts.StartTimeFrom != nil {
		filter[constants.MongoFieldTimestamp] = bson.M{"$gte": *opts.StartTimeFrom}
	}

	// No else needed: optional operation (only add filter if specified)
	if opts.StartTimeTo != nil {
		// No else needed: optional operation (merge with existing filter or create new)
		if existingFilter, ok := filter[constants.MongoFieldTimestamp].(bson.M); ok {
			existingFilter["$lte"] = *opts.StartTimeTo
		} else {
			filter[constants.MongoFieldTimestamp] = bson.M{"$lte": *opts.StartTimeTo}
		}
	}

	// No else needed: optional operation (only add filter if specified)
	if opts.AdminAssisted != nil {
		filter[constants.MongoFieldAdminAssisted] = *opts.AdminAssisted
	}

	// No else needed: optional operation (only add filter if specified)
	if opts.Active != nil {
		// No else needed: conditional operation (different filter based on value)
		if *opts.Active {
			// Active sessions have no endTs
			filter[constants.MongoFieldEndTime] = bson.M{"$exists": false}
		} else {
			// Ended sessions have endTs
			filter[constants.MongoFieldEndTime] = bson.M{"$exists": true}
		}
	}

	// Build sort
	sortOrder := -1 // descending
	// No else needed: optional operation (only change if ascending)
	if opts.SortOrder == constants.SortOrderAsc {
		sortOrder = 1
	}

	sortField := constants.MongoFieldTimestamp
	switch opts.SortBy {
	case constants.SortByEndTime:
		sortField = constants.MongoFieldEndTime
	case constants.SortByMessageCount:
		// We'll need to sort by array size, which requires aggregation
		// For now, we'll sort by ts and handle message_count in application
		sortField = constants.MongoFieldTimestamp
	case constants.SortByTotalTokens:
		sortField = constants.MongoFieldTotalTokens
	case constants.SortByUserID:
		sortField = constants.MongoFieldUserID
	default:
		sortField = constants.MongoFieldTimestamp
	}

	// Build find options
	findOptions := options.Find()
	findOptions.SetSort(bson.D{{Key: sortField, Value: sortOrder}})
	findOptions.SetLimit(int64(opts.Limit))
	findOptions.SetSkip(int64(opts.Offset))

	// Execute query
	cursor, err := s.collection.Find(ctx, filter, findOptions)
	// No else needed: early return pattern (guard clause)
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions with options: %w", err)
	}
	defer cursor.Close(ctx)

	// Decode results
	var sessions []*SessionMetadata
	for cursor.Next(ctx) {
		var doc SessionDocument
		// No else needed: early return pattern (guard clause)
		if err := cursor.Decode(&doc); err != nil {
			return nil, fmt.Errorf("failed to decode session document: %w", err)
		}

		// Determine last message time
		lastMessageTime := doc.StartTime
		// No else needed: optional operation (only update if messages exist)
		if len(doc.Messages) > 0 {
			lastMessageTime = doc.Messages[len(doc.Messages)-1].Timestamp
		}

		metadata := &SessionMetadata{
			ID:              doc.ID,
			UserID:          doc.UserID,
			Name:            doc.Name,
			LastMessageTime: lastMessageTime,
			MessageCount:    len(doc.Messages),
			AdminAssisted:   doc.AdminAssisted,
			StartTime:       doc.StartTime,
			EndTime:         doc.EndTime,
			TotalTokens:     doc.TotalTokens,
			MaxResponseTime: doc.MaxResponseTime,
			AvgResponseTime: doc.AvgResponseTime,
		}

		sessions = append(sessions, metadata)
	}

	// No else needed: early return pattern (guard clause)
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("cursor error: %w", err)
	}

	// If sorting by message_count, sort in application
	// No else needed: optional operation (only sort if requested)
	if opts.SortBy == constants.SortByMessageCount {
		sortByMessageCount(sessions, opts.SortOrder == constants.SortOrderAsc)
	}

	return sessions, nil
}

// sortByMessageCount sorts sessions by message count in place using O(n log n) algorithm
func sortByMessageCount(sessions []*SessionMetadata, ascending bool) {
	sort.Slice(sessions, func(i, j int) bool {
		if ascending {
			return sessions[i].MessageCount < sessions[j].MessageCount
		}
		return sessions[i].MessageCount > sessions[j].MessageCount
	})
}

// GetSessionMetrics calculates aggregated metrics for all sessions within a time period
// Returns metrics including total sessions, active sessions, token usage, and response times
func (s *StorageService) GetSessionMetrics(startTime, endTime time.Time) (*Metrics, error) {
	// No else needed: early return pattern (guard clause)
	if endTime.Before(startTime) {
		return nil, errors.New("end time must be after start time")
	}

	ctx, cancel := util.NewTimeoutContext(constants.MetricsTimeout)
	defer cancel()

	// Build query filter for sessions within time range
	filter := bson.M{
		constants.MongoFieldTimestamp: bson.M{
			"$gte": startTime,
			"$lte": endTime,
		},
	}

	// Execute query using gomongo
	cursor, err := s.collection.Find(ctx, filter)
	// No else needed: early return pattern (guard clause)
	if err != nil {
		return nil, fmt.Errorf("failed to get session metrics: %w", err)
	}
	defer cursor.Close(ctx)

	// Initialize metrics
	metrics := &Metrics{
		TotalSessions:      0,
		ActiveSessions:     0,
		AvgConcurrent:      0,
		MaxConcurrent:      0,
		TotalTokens:        0,
		AvgResponseTime:    0,
		MaxResponseTime:    0,
		AdminAssistedCount: 0,
	}

	// Track concurrent sessions over time for average calculation
	// Map of timestamp -> count of active sessions at that time
	concurrentMap := make(map[int64]int)

	var totalResponseTime int64
	var responseTimeCount int

	// Process each session
	for cursor.Next(ctx) {
		var doc SessionDocument
		// No else needed: early return pattern (guard clause)
		if err := cursor.Decode(&doc); err != nil {
			return nil, fmt.Errorf("failed to decode session document: %w", err)
		}

		metrics.TotalSessions++

		// Count active sessions (no end time)
		// No else needed: optional operation (only count if active)
		if doc.EndTime == nil {
			metrics.ActiveSessions++
		}

		// Count admin-assisted sessions
		// No else needed: optional operation (only count if assisted)
		if doc.AdminAssisted {
			metrics.AdminAssistedCount++
		}

		// Aggregate token usage
		metrics.TotalTokens += doc.TotalTokens

		// Track max response time
		// No else needed: optional operation (only update if larger)
		if doc.MaxResponseTime > metrics.MaxResponseTime {
			metrics.MaxResponseTime = doc.MaxResponseTime
		}

		// Aggregate average response times
		// No else needed: optional operation (only aggregate if exists)
		if doc.AvgResponseTime > 0 {
			totalResponseTime += doc.AvgResponseTime
			responseTimeCount++
		}

		// Track concurrent sessions
		// Increment at start time, decrement at end time
		startUnix := doc.StartTime.Unix()
		concurrentMap[startUnix]++

		// No else needed: optional operation (only decrement if session ended)
		if doc.EndTime != nil {
			endUnix := doc.EndTime.Unix()
			concurrentMap[endUnix]--
		}
	}

	// No else needed: early return pattern (guard clause)
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("cursor error: %w", err)
	}

	// Calculate average response time
	// No else needed: optional operation (only calculate if data exists)
	if responseTimeCount > 0 {
		metrics.AvgResponseTime = totalResponseTime / int64(responseTimeCount)
	}

	// Calculate max concurrent and average concurrent sessions
	// No else needed: optional operation (only calculate if data exists)
	if len(concurrentMap) > 0 {
		// Sort timestamps
		timestamps := make([]int64, 0, len(concurrentMap))
		for ts := range concurrentMap {
			timestamps = append(timestamps, ts)
		}

		// Sort timestamps using efficient O(n log n) algorithm
		sort.Slice(timestamps, func(i, j int) bool {
			return timestamps[i] < timestamps[j]
		})

		// Calculate running concurrent count
		currentConcurrent := 0
		var totalConcurrent int64
		sampleCount := 0

		for _, ts := range timestamps {
			currentConcurrent += concurrentMap[ts]
			// No else needed: optional operation (only update if larger)
			if currentConcurrent > metrics.MaxConcurrent {
				metrics.MaxConcurrent = currentConcurrent
			}
			totalConcurrent += int64(currentConcurrent)
			sampleCount++
		}

		// No else needed: optional operation (only calculate if samples exist)
		if sampleCount > 0 {
			metrics.AvgConcurrent = float64(totalConcurrent) / float64(sampleCount)
		}
	}

	return metrics, nil
}

// GetTokenUsage calculates the total token usage across all sessions within a time period
func (s *StorageService) GetTokenUsage(startTime, endTime time.Time) (int, error) {
	// No else needed: early return pattern (guard clause)
	if endTime.Before(startTime) {
		return 0, errors.New("end time must be after start time")
	}

	ctx, cancel := util.NewTimeoutContext(constants.DefaultContextTimeout)
	defer cancel()

	// Use MongoDB aggregation pipeline to sum token usage
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{
			constants.MongoFieldTimestamp: bson.M{
				"$gte": startTime,
				"$lte": endTime,
			},
		}}},
		{{Key: "$group", Value: bson.M{
			"_id":         nil,
			"totalTokens": bson.M{"$sum": "$totalTokens"},
		}}},
	}

	cursor, err := s.collection.Aggregate(ctx, pipeline)
	// No else needed: early return pattern (guard clause)
	if err != nil {
		return 0, fmt.Errorf("failed to aggregate token usage: %w", err)
	}
	defer cursor.Close(ctx)

	// Extract result
	var result struct {
		TotalTokens int `bson:"totalTokens"`
	}

	// No else needed: optional operation (only decode if result exists)
	if cursor.Next(ctx) {
		// No else needed: early return pattern (guard clause)
		if err := cursor.Decode(&result); err != nil {
			return 0, fmt.Errorf("failed to decode aggregation result: %w", err)
		}
	}

	// No else needed: early return pattern (guard clause)
	if err := cursor.Err(); err != nil {
		return 0, fmt.Errorf("cursor error: %w", err)
	}

	return result.TotalTokens, nil
}

// retryOperation executes an operation with retry logic for transient errors
// Uses exponential backoff with configurable parameters
func (s *StorageService) retryOperation(ctx context.Context, operation string, fn func() error) error {
	var lastErr error
	delay := defaultRetryConfig.initialDelay

	for attempt := 1; attempt <= defaultRetryConfig.maxAttempts; attempt++ {
		err := fn()
		// No else needed: early return pattern (guard clause - success case)
		if err == nil {
			return nil
		}

		// Check if error is retryable
		// No else needed: early return pattern (guard clause - non-retryable error)
		if !isRetryableError(err) {
			return err
		}

		lastErr = err

		// No else needed: optional operation (only retry if attempts remain)
		if attempt < defaultRetryConfig.maxAttempts {
			s.logger.Warn("MongoDB operation failed, retrying",
				"operation", operation,
				"attempt", attempt,
				"max_attempts", defaultRetryConfig.maxAttempts,
				"delay", delay,
				"error", err)

			// Sleep with context awareness
			select {
			case <-time.After(delay):
				// Continue to next attempt
			case <-ctx.Done():
				return fmt.Errorf("operation cancelled during retry: %w", ctx.Err())
			}

			// Exponential backoff
			delay = time.Duration(float64(delay) * defaultRetryConfig.multiplier)
			// No else needed: optional operation (only cap if exceeds max)
			if delay > defaultRetryConfig.maxDelay {
				delay = defaultRetryConfig.maxDelay
			}
		}
	}

	return fmt.Errorf("operation failed after %d attempts: %w",
		defaultRetryConfig.maxAttempts, lastErr)
}
