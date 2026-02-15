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
	"time"

	"github.com/real-rm/chatbox/internal/session"
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

// StorageService manages conversation persistence in MongoDB using gomongo
type StorageService struct {
	mongo         *gomongo.Mongo
	collection    *gomongo.MongoCollection
	logger        *golog.Logger
	encryptionKey []byte // Key for encrypting sensitive fields
}

// SessionDocument represents a session stored in MongoDB
type SessionDocument struct {
	ID               string            `bson:"_id"`
	UserID           string            `bson:"user_id"`
	Name             string            `bson:"name"`
	ModelID          string            `bson:"model_id"`
	Messages         []MessageDocument `bson:"messages"`
	StartTime        time.Time         `bson:"start_time"`
	EndTime          *time.Time        `bson:"end_time,omitempty"`
	Duration         int64             `bson:"duration"` // seconds
	AdminAssisted    bool              `bson:"admin_assisted"`
	AssistingAdminID string            `bson:"assisting_admin_id,omitempty"`
	HelpRequested    bool              `bson:"help_requested"`
	TotalTokens      int               `bson:"total_tokens"`
	MaxResponseTime  int64             `bson:"max_response_time"` // milliseconds
	AvgResponseTime  int64             `bson:"avg_response_time"` // milliseconds
	CreatedAt        time.Time         `bson:"_ts,omitempty"`     // gomongo automatic timestamp
	ModifiedAt       time.Time         `bson:"_mt,omitempty"`     // gomongo automatic timestamp
}

// MessageDocument represents a message stored in MongoDB
type MessageDocument struct {
	Content   string            `bson:"content"`
	Timestamp time.Time         `bson:"timestamp"`
	Sender    string            `bson:"sender"` // "user", "ai", "admin"
	FileID    string            `bson:"file_id,omitempty"`
	FileURL   string            `bson:"file_url,omitempty"`
	Metadata  map[string]string `bson:"metadata,omitempty"`
}

// SessionMetadata represents summary information about a session
type SessionMetadata struct {
	ID              string
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

// CreateSession creates a new session document in MongoDB
func (s *StorageService) CreateSession(sess *session.Session) error {
	if sess == nil {
		return ErrInvalidSession
	}

	if sess.ID == "" {
		return ErrInvalidSessionID
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Convert session to document
	doc := s.sessionToDocument(sess)

	// Insert document using gomongo (automatically adds _ts and _mt timestamps)
	_, err := s.collection.InsertOne(ctx, doc)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	return nil
}

// UpdateSession updates an existing session document in MongoDB
func (s *StorageService) UpdateSession(sess *session.Session) error {
	if sess == nil {
		return ErrInvalidSession
	}

	if sess.ID == "" {
		return ErrInvalidSessionID
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Convert session to document
	doc := s.sessionToDocument(sess)

	// Update document using gomongo (automatically updates _mt timestamp)
	filter := bson.M{"_id": sess.ID}
	update := bson.M{"$set": doc}

	result, err := s.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	if result.MatchedCount == 0 {
		return ErrSessionNotFound
	}

	return nil
}

// GetSession retrieves a session from MongoDB by ID
func (s *StorageService) GetSession(sessionID string) (*session.Session, error) {
	if sessionID == "" {
		return nil, ErrInvalidSessionID
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Find document using gomongo
	filter := bson.M{"_id": sessionID}
	var doc SessionDocument

	result := s.collection.FindOne(ctx, filter)
	err := result.Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	// Convert document to session
	sess := s.documentToSession(&doc)

	return sess, nil
}

// sessionToDocument converts a Session to a SessionDocument
func (s *StorageService) sessionToDocument(sess *session.Session) *SessionDocument {
	// Note: Session fields are accessed directly without locking
	// The caller should ensure thread-safety if needed

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
	if len(sess.ResponseTimes) > 0 {
		var total time.Duration
		maxDuration := sess.ResponseTimes[0]

		for _, rt := range sess.ResponseTimes {
			total += rt
			if rt > maxDuration {
				maxDuration = rt
			}
		}

		maxResponseTime = maxDuration.Milliseconds()
		avgResponseTime = (total / time.Duration(len(sess.ResponseTimes))).Milliseconds()
	}

	return &SessionDocument{
		ID:               sess.ID,
		UserID:           sess.UserID,
		Name:             sess.Name,
		ModelID:          sess.ModelID,
		Messages:         messages,
		StartTime:        sess.StartTime,
		EndTime:          sess.EndTime,
		Duration:         duration,
		AdminAssisted:    sess.AdminAssisted,
		AssistingAdminID: sess.AssistingAdminID,
		HelpRequested:    sess.HelpRequested,
		TotalTokens:      sess.TotalTokens,
		MaxResponseTime:  maxResponseTime,
		AvgResponseTime:  avgResponseTime,
	}
}

// documentToSession converts a SessionDocument to a Session
func (s *StorageService) documentToSession(doc *SessionDocument) *session.Session {
	// Convert messages and decrypt content
	messages := make([]*session.Message, len(doc.Messages))
	for i, msg := range doc.Messages {
		content := msg.Content
		// Decrypt content if encryption key is provided
		if len(s.encryptionKey) > 0 {
			decrypted, err := s.decrypt(msg.Content)
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
		AssistingAdminName: "", // Not stored in document
		TotalTokens:        doc.TotalTokens,
		ResponseTimes:      responseTimes,
	}
}

// AddMessage adds a message to an existing session and persists it immediately
func (s *StorageService) AddMessage(sessionID string, msg *session.Message) error {
	if sessionID == "" {
		return ErrInvalidSessionID
	}

	if msg == nil {
		return errors.New("message cannot be nil")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
	if len(s.encryptionKey) > 0 {
		encrypted, err := s.encrypt(msgDoc.Content)
		if err != nil {
			return fmt.Errorf("failed to encrypt message content: %w", err)
		}
		msgDoc.Content = encrypted
	}

	// Push message to messages array using gomongo (automatically updates _mt)
	filter := bson.M{"_id": sessionID}
	update := bson.M{
		"$push": bson.M{"messages": msgDoc},
		"$set":  bson.M{"last_activity": time.Now()},
	}

	result, err := s.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to add message: %w", err)
	}

	if result.MatchedCount == 0 {
		return ErrSessionNotFound
	}

	return nil
}

// EndSession updates the session with end timestamp and duration
func (s *StorageService) EndSession(sessionID string, endTime time.Time) error {
	if sessionID == "" {
		return ErrInvalidSessionID
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Get the session to calculate duration
	var doc SessionDocument
	err := s.collection.FindOne(ctx, bson.M{"_id": sessionID}).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return ErrSessionNotFound
		}
		return fmt.Errorf("failed to get session: %w", err)
	}

	// Calculate duration
	duration := int64(endTime.Sub(doc.StartTime).Seconds())

	// Update session with end time and duration using gomongo (automatically updates _mt)
	filter := bson.M{"_id": sessionID}
	update := bson.M{
		"$set": bson.M{
			"end_time": endTime,
			"duration": duration,
		},
	}

	result, err := s.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to end session: %w", err)
	}

	if result.MatchedCount == 0 {
		return ErrSessionNotFound
	}

	return nil
}

// encrypt encrypts data using AES-256-GCM
func (s *StorageService) encrypt(plaintext string) (string, error) {
	if len(s.encryptionKey) == 0 {
		return plaintext, nil
	}

	block, err := aes.NewCipher(s.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	// Create nonce
	nonce := make([]byte, gcm.NonceSize())
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
	if len(s.encryptionKey) == 0 {
		return ciphertext, nil
	}

	// Decode from base64
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	block, err := aes.NewCipher(s.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	// Extract nonce and ciphertext
	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]

	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}

	return string(plaintext), nil
}

// ListUserSessions retrieves all sessions for a user ordered by last activity (most recent first)
// The limit parameter controls the maximum number of sessions to return (0 = no limit)
func (s *StorageService) ListUserSessions(userID string, limit int) ([]*SessionMetadata, error) {
	if userID == "" {
		return nil, errors.New("user ID cannot be empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Build query filter
	filter := bson.M{"user_id": userID}

	// Build find options with sorting by start_time (descending)
	findOptions := options.Find()
	findOptions.SetSort(bson.D{{Key: "start_time", Value: -1}})

	if limit > 0 {
		findOptions.SetLimit(int64(limit))
	}

	// Execute query using gomongo
	cursor, err := s.collection.Find(ctx, filter, findOptions)
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

// GetSessionMetrics calculates aggregated metrics for all sessions within a time period
// Returns metrics including total sessions, active sessions, token usage, and response times
func (s *StorageService) GetSessionMetrics(startTime, endTime time.Time) (*Metrics, error) {
	if endTime.Before(startTime) {
		return nil, errors.New("end time must be after start time")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Build query filter for sessions within time range
	filter := bson.M{
		"start_time": bson.M{
			"$gte": startTime,
			"$lte": endTime,
		},
	}

	// Execute query using gomongo
	cursor, err := s.collection.Find(ctx, filter)
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
		if err := cursor.Decode(&doc); err != nil {
			return nil, fmt.Errorf("failed to decode session document: %w", err)
		}

		metrics.TotalSessions++

		// Count active sessions (no end time)
		if doc.EndTime == nil {
			metrics.ActiveSessions++
		}

		// Count admin-assisted sessions
		if doc.AdminAssisted {
			metrics.AdminAssistedCount++
		}

		// Aggregate token usage
		metrics.TotalTokens += doc.TotalTokens

		// Track max response time
		if doc.MaxResponseTime > metrics.MaxResponseTime {
			metrics.MaxResponseTime = doc.MaxResponseTime
		}

		// Aggregate average response times
		if doc.AvgResponseTime > 0 {
			totalResponseTime += doc.AvgResponseTime
			responseTimeCount++
		}

		// Track concurrent sessions
		// Increment at start time, decrement at end time
		startUnix := doc.StartTime.Unix()
		concurrentMap[startUnix]++

		if doc.EndTime != nil {
			endUnix := doc.EndTime.Unix()
			concurrentMap[endUnix]--
		}
	}

	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("cursor error: %w", err)
	}

	// Calculate average response time
	if responseTimeCount > 0 {
		metrics.AvgResponseTime = totalResponseTime / int64(responseTimeCount)
	}

	// Calculate max concurrent and average concurrent sessions
	if len(concurrentMap) > 0 {
		// Sort timestamps
		timestamps := make([]int64, 0, len(concurrentMap))
		for ts := range concurrentMap {
			timestamps = append(timestamps, ts)
		}

		// Simple sort (bubble sort for small datasets)
		for i := 0; i < len(timestamps); i++ {
			for j := i + 1; j < len(timestamps); j++ {
				if timestamps[i] > timestamps[j] {
					timestamps[i], timestamps[j] = timestamps[j], timestamps[i]
				}
			}
		}

		// Calculate running concurrent count
		currentConcurrent := 0
		var totalConcurrent int64
		sampleCount := 0

		for _, ts := range timestamps {
			currentConcurrent += concurrentMap[ts]
			if currentConcurrent > metrics.MaxConcurrent {
				metrics.MaxConcurrent = currentConcurrent
			}
			totalConcurrent += int64(currentConcurrent)
			sampleCount++
		}

		if sampleCount > 0 {
			metrics.AvgConcurrent = float64(totalConcurrent) / float64(sampleCount)
		}
	}

	return metrics, nil
}

// GetTokenUsage calculates the total token usage across all sessions within a time period
func (s *StorageService) GetTokenUsage(startTime, endTime time.Time) (int, error) {
	if endTime.Before(startTime) {
		return 0, errors.New("end time must be after start time")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Use MongoDB aggregation pipeline to sum token usage
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{
			"start_time": bson.M{
				"$gte": startTime,
				"$lte": endTime,
			},
		}}},
		{{Key: "$group", Value: bson.M{
			"_id":          nil,
			"total_tokens": bson.M{"$sum": "$total_tokens"},
		}}},
	}

	cursor, err := s.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return 0, fmt.Errorf("failed to aggregate token usage: %w", err)
	}
	defer cursor.Close(ctx)

	// Extract result
	var result struct {
		TotalTokens int `bson:"total_tokens"`
	}

	if cursor.Next(ctx) {
		if err := cursor.Decode(&result); err != nil {
			return 0, fmt.Errorf("failed to decode aggregation result: %w", err)
		}
	}

	if err := cursor.Err(); err != nil {
		return 0, fmt.Errorf("cursor error: %w", err)
	}

	return result.TotalTokens, nil
}
