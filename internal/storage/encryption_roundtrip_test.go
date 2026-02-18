package storage

import (
	"testing"
	"time"

	"github.com/real-rm/chatbox/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEncryptionRoundTrip_CompleteFlow tests the full encryption/decryption flow
// This test verifies that:
// 1. Messages can be encrypted with a valid key
// 2. Encrypted messages can be decrypted back to original content
// 3. The encryption is transparent to the application logic
// 4. Multiple messages can be encrypted/decrypted correctly
func TestEncryptionRoundTrip_CompleteFlow(t *testing.T) {
	// Create a 32-byte encryption key for AES-256
	encryptionKey := []byte("12345678901234567890123456789012")
	require.Len(t, encryptionKey, 32, "Encryption key must be 32 bytes for AES-256")

	service := &StorageService{
		encryptionKey: encryptionKey,
	}

	// Test cases with various message contents
	testCases := []struct {
		name    string
		content string
	}{
		{
			name:    "Simple message",
			content: "Hello, this is a test message",
		},
		{
			name:    "Message with special characters",
			content: "Special chars: !@#$%^&*()_+-=[]{}|;':\",./<>?",
		},
		{
			name:    "Message with unicode",
			content: "Unicode: 你好世界 🌍 مرحبا العالم",
		},
		{
			name: "Long message",
			content: "This is a much longer message that contains multiple sentences. " +
				"It should still be encrypted and decrypted correctly. " +
				"The encryption should handle arbitrary length messages without issues. " +
				"Let's add even more text to make sure it works with longer content. " +
				"Encryption is important for protecting sensitive user data at rest.",
		},
		{
			name:    "Empty message",
			content: "",
		},
		{
			name:    "Message with newlines",
			content: "Line 1\nLine 2\nLine 3\n",
		},
		{
			name:    "Message with tabs",
			content: "Column1\tColumn2\tColumn3",
		},
		{
			name:    "Sensitive data",
			content: "User password: secret123, Credit card: 1234-5678-9012-3456",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Step 1: Encrypt the message
			encrypted, err := service.encrypt(tc.content)
			require.NoError(t, err, "Encryption should not fail")

			// Verify encryption actually happened (unless empty)
			if tc.content != "" {
				assert.NotEqual(t, tc.content, encrypted, "Encrypted content should differ from plaintext")
			}

			// Step 2: Decrypt the message
			decrypted, err := service.decrypt(encrypted)
			require.NoError(t, err, "Decryption should not fail")

			// Step 3: Verify round-trip integrity
			assert.Equal(t, tc.content, decrypted, "Decrypted content should match original")
		})
	}
}

// TestEncryptionRoundTrip_SessionConversion tests encryption through session document conversion
// This verifies that the sessionToDocument and documentToSession methods properly handle encryption
func TestEncryptionRoundTrip_SessionConversion(t *testing.T) {
	// Create a 32-byte encryption key for AES-256
	encryptionKey := []byte("12345678901234567890123456789012")
	service := &StorageService{
		encryptionKey: encryptionKey,
	}

	// Create a session with multiple messages
	now := time.Now()
	originalSession := &session.Session{
		ID:      "test-session-encryption",
		UserID:  "user-123",
		Name:    "Encryption Test Session",
		ModelID: "gpt-4",
		Messages: []*session.Message{
			{
				Content:   "First message with sensitive data",
				Timestamp: now,
				Sender:    "user",
				FileID:    "",
				FileURL:   "",
				Metadata:  map[string]string{"type": "text"},
			},
			{
				Content:   "Second message: password123",
				Timestamp: now.Add(time.Minute),
				Sender:    "ai",
				FileID:    "",
				FileURL:   "",
				Metadata:  map[string]string{"type": "response"},
			},
			{
				Content:   "Third message with unicode: 你好 🌍",
				Timestamp: now.Add(2 * time.Minute),
				Sender:    "user",
				FileID:    "",
				FileURL:   "",
				Metadata:  nil,
			},
		},
		StartTime:     now,
		LastActivity:  now,
		EndTime:       nil,
		IsActive:      true,
		HelpRequested: false,
		AdminAssisted: false,
		TotalTokens:   150,
		ResponseTimes: []time.Duration{time.Second, 2 * time.Second},
	}

	// Step 1: Convert session to document (this should NOT encrypt yet, as encryption happens in AddMessage)
	doc := service.sessionToDocument(originalSession)
	require.NotNil(t, doc)
	assert.Equal(t, originalSession.ID, doc.ID)
	assert.Len(t, doc.Messages, 3)

	// Manually encrypt the messages in the document (simulating what AddMessage does)
	for i := range doc.Messages {
		encrypted, err := service.encrypt(doc.Messages[i].Content)
		require.NoError(t, err)
		doc.Messages[i].Content = encrypted
	}

	// Verify messages are encrypted
	assert.NotEqual(t, "First message with sensitive data", doc.Messages[0].Content)
	assert.NotEqual(t, "Second message: password123", doc.Messages[1].Content)
	assert.NotEqual(t, "Third message with unicode: 你好 🌍", doc.Messages[2].Content)

	// Step 2: Convert document back to session (this should decrypt)
	retrievedSession := service.documentToSession(doc)
	require.NotNil(t, retrievedSession)

	// Step 3: Verify all messages were decrypted correctly
	assert.Equal(t, originalSession.ID, retrievedSession.ID)
	assert.Equal(t, originalSession.UserID, retrievedSession.UserID)
	assert.Len(t, retrievedSession.Messages, 3)

	assert.Equal(t, "First message with sensitive data", retrievedSession.Messages[0].Content)
	assert.Equal(t, "user", retrievedSession.Messages[0].Sender)

	assert.Equal(t, "Second message: password123", retrievedSession.Messages[1].Content)
	assert.Equal(t, "ai", retrievedSession.Messages[1].Sender)

	assert.Equal(t, "Third message with unicode: 你好 🌍", retrievedSession.Messages[2].Content)
	assert.Equal(t, "user", retrievedSession.Messages[2].Sender)
}

// TestEncryptionRoundTrip_MultipleKeys tests that different keys produce different ciphertexts
func TestEncryptionRoundTrip_MultipleKeys(t *testing.T) {
	key1 := []byte("12345678901234567890123456789012")
	key2 := []byte("abcdefghijklmnopqrstuvwxyz123456")

	service1 := &StorageService{encryptionKey: key1}
	service2 := &StorageService{encryptionKey: key2}

	plaintext := "Sensitive message content"

	// Encrypt with key1
	encrypted1, err := service1.encrypt(plaintext)
	require.NoError(t, err)

	// Encrypt with key2
	encrypted2, err := service2.encrypt(plaintext)
	require.NoError(t, err)

	// Verify different keys produce different ciphertexts
	assert.NotEqual(t, encrypted1, encrypted2, "Different keys should produce different ciphertexts")

	// Verify each can decrypt its own ciphertext
	decrypted1, err := service1.decrypt(encrypted1)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted1)

	decrypted2, err := service2.decrypt(encrypted2)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted2)

	// Verify cross-decryption fails (wrong key)
	_, err = service1.decrypt(encrypted2)
	assert.Error(t, err, "Decrypting with wrong key should fail")

	_, err = service2.decrypt(encrypted1)
	assert.Error(t, err, "Decrypting with wrong key should fail")
}

// TestEncryptionRoundTrip_NonDeterministic tests that encryption is non-deterministic
// (same plaintext produces different ciphertexts due to random nonce)
func TestEncryptionRoundTrip_NonDeterministic(t *testing.T) {
	encryptionKey := []byte("12345678901234567890123456789012")
	service := &StorageService{encryptionKey: encryptionKey}

	plaintext := "Same message encrypted multiple times"

	// Encrypt the same message multiple times
	encrypted1, err := service.encrypt(plaintext)
	require.NoError(t, err)

	encrypted2, err := service.encrypt(plaintext)
	require.NoError(t, err)

	encrypted3, err := service.encrypt(plaintext)
	require.NoError(t, err)

	// Verify each encryption produces different ciphertext (due to random nonce)
	assert.NotEqual(t, encrypted1, encrypted2, "Same plaintext should produce different ciphertexts")
	assert.NotEqual(t, encrypted2, encrypted3, "Same plaintext should produce different ciphertexts")
	assert.NotEqual(t, encrypted1, encrypted3, "Same plaintext should produce different ciphertexts")

	// Verify all can be decrypted to the same plaintext
	decrypted1, err := service.decrypt(encrypted1)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted1)

	decrypted2, err := service.decrypt(encrypted2)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted2)

	decrypted3, err := service.decrypt(encrypted3)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted3)
}

// TestEncryptionRoundTrip_WithoutKey tests that encryption is bypassed when no key is provided
func TestEncryptionRoundTrip_WithoutKey(t *testing.T) {
	service := &StorageService{
		encryptionKey: nil, // No encryption key
	}

	plaintext := "This message should not be encrypted"

	// Encrypt without key (should return plaintext)
	encrypted, err := service.encrypt(plaintext)
	require.NoError(t, err)
	assert.Equal(t, plaintext, encrypted, "Without key, encrypt should return plaintext")

	// Decrypt without key (should return input as-is)
	decrypted, err := service.decrypt(encrypted)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted, "Without key, decrypt should return input as-is")
}

// TestEncryptionRoundTrip_KeyLength tests that valid AES key lengths are accepted
// AES supports 16-byte (AES-128), 24-byte (AES-192), and 32-byte (AES-256) keys
func TestEncryptionRoundTrip_KeyLength(t *testing.T) {
	testCases := []struct {
		name      string
		key       []byte
		shouldErr bool
	}{
		{
			name:      "Valid 32-byte key (AES-256)",
			key:       []byte("12345678901234567890123456789012"),
			shouldErr: false,
		},
		{
			name:      "Valid 16-byte key (AES-128)",
			key:       []byte("1234567890123456"),
			shouldErr: false,
		},
		{
			name:      "Valid 24-byte key (AES-192)",
			key:       []byte("123456789012345678901234"),
			shouldErr: false,
		},
		{
			name:      "Invalid 31-byte key",
			key:       []byte("1234567890123456789012345678901"),
			shouldErr: true,
		},
		{
			name:      "Invalid 33-byte key",
			key:       []byte("123456789012345678901234567890123"),
			shouldErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			service := &StorageService{encryptionKey: tc.key}
			plaintext := "Test message"

			encrypted, err := service.encrypt(plaintext)

			if tc.shouldErr {
				assert.Error(t, err, "Encryption with invalid key length should fail")
			} else {
				assert.NoError(t, err, "Encryption with valid key length should succeed")

				// Verify decryption works
				decrypted, err := service.decrypt(encrypted)
				assert.NoError(t, err)
				assert.Equal(t, plaintext, decrypted)
			}
		})
	}
}
