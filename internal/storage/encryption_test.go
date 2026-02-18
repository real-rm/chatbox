package storage

import (
	"crypto/aes"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEncryption_KeySizes tests encryption with all valid AES key sizes
// Validates requirement 6.3: Test encryption with various key sizes
func TestEncryption_KeySizes(t *testing.T) {
	testCases := []struct {
		name      string
		keySize   int
		shouldErr bool
		aesType   string
	}{
		{
			name:      "AES-128 (16 bytes)",
			keySize:   16,
			shouldErr: false,
			aesType:   "AES-128",
		},
		{
			name:      "AES-192 (24 bytes)",
			keySize:   24,
			shouldErr: false,
			aesType:   "AES-192",
		},
		{
			name:      "AES-256 (32 bytes)",
			keySize:   32,
			shouldErr: false,
			aesType:   "AES-256",
		},
		{
			name:      "Invalid 8 bytes",
			keySize:   8,
			shouldErr: true,
			aesType:   "Invalid",
		},
		{
			name:      "Invalid 15 bytes",
			keySize:   15,
			shouldErr: true,
			aesType:   "Invalid",
		},
		{
			name:      "Invalid 17 bytes",
			keySize:   17,
			shouldErr: true,
			aesType:   "Invalid",
		},
		{
			name:      "Invalid 31 bytes",
			keySize:   31,
			shouldErr: true,
			aesType:   "Invalid",
		},
		{
			name:      "Invalid 33 bytes",
			keySize:   33,
			shouldErr: true,
			aesType:   "Invalid",
		},
		{
			name:      "Invalid 64 bytes",
			keySize:   64,
			shouldErr: true,
			aesType:   "Invalid",
		},
	}

	plaintext := "Test message for encryption"

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create key of specified size
			key := make([]byte, tc.keySize)
			for i := range key {
				key[i] = byte(i % 256)
			}

			service := &StorageService{
				encryptionKey: key,
			}

			// Attempt encryption
			encrypted, err := service.encrypt(plaintext)

			if tc.shouldErr {
				assert.Error(t, err, "Encryption with %s key should fail", tc.aesType)
				assert.Contains(t, err.Error(), "failed to create cipher", "Error should mention cipher creation failure")
			} else {
				require.NoError(t, err, "Encryption with %s key should succeed", tc.aesType)
				assert.NotEqual(t, plaintext, encrypted, "Encrypted text should differ from plaintext")

				// Verify decryption works
				decrypted, err := service.decrypt(encrypted)
				require.NoError(t, err, "Decryption with %s key should succeed", tc.aesType)
				assert.Equal(t, plaintext, decrypted, "Decrypted text should match original")
			}
		})
	}
}

// TestDecryption_Failures tests various decryption failure scenarios
// Validates requirement 6.3: Test decryption failures
func TestDecryption_Failures(t *testing.T) {
	validKey := []byte("12345678901234567890123456789012") // 32 bytes
	service := &StorageService{
		encryptionKey: validKey,
	}

	testCases := []struct {
		name          string
		ciphertext    string
		expectedError string
	}{
		{
			name:          "Invalid base64",
			ciphertext:    "not-valid-base64!@#$%^&*()",
			expectedError: "failed to decode base64",
		},
		{
			name:          "Empty ciphertext",
			ciphertext:    "",
			expectedError: "ciphertext too short",
		},
		{
			name:          "Valid base64 but too short",
			ciphertext:    base64.StdEncoding.EncodeToString([]byte("short")),
			expectedError: "ciphertext too short",
		},
		{
			name:          "Valid base64 but corrupted data",
			ciphertext:    base64.StdEncoding.EncodeToString([]byte("this is corrupted ciphertext data that is long enough")),
			expectedError: "failed to decrypt",
		},
		{
			name:          "Tampered nonce",
			ciphertext:    createTamperedCiphertext(t, service, "tamper nonce"),
			expectedError: "failed to decrypt",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.decrypt(tc.ciphertext)
			require.Error(t, err, "Decryption should fail for %s", tc.name)
			assert.Contains(t, err.Error(), tc.expectedError, "Error message should contain expected text")
		})
	}
}

// TestDecryption_WrongKey tests decryption with incorrect keys
// Validates requirement 6.3: Test decryption failures
func TestDecryption_WrongKey(t *testing.T) {
	key1 := []byte("12345678901234567890123456789012")
	key2 := []byte("abcdefghijklmnopqrstuvwxyz123456")
	key3 := []byte("00000000000000000000000000000000")

	service1 := &StorageService{encryptionKey: key1}
	service2 := &StorageService{encryptionKey: key2}
	service3 := &StorageService{encryptionKey: key3}

	plaintext := "Secret message"

	// Encrypt with key1
	encrypted, err := service1.encrypt(plaintext)
	require.NoError(t, err)

	// Try to decrypt with key2 (wrong key)
	_, err = service2.decrypt(encrypted)
	assert.Error(t, err, "Decryption with wrong key should fail")
	assert.Contains(t, err.Error(), "failed to decrypt", "Error should indicate decryption failure")

	// Try to decrypt with key3 (another wrong key)
	_, err = service3.decrypt(encrypted)
	assert.Error(t, err, "Decryption with wrong key should fail")
	assert.Contains(t, err.Error(), "failed to decrypt", "Error should indicate decryption failure")

	// Verify correct key still works
	decrypted, err := service1.decrypt(encrypted)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

// TestEncryption_RoundTrip tests complete encryption/decryption cycles
// Validates requirement 6.3: Test encryption round-trip
func TestEncryption_RoundTrip(t *testing.T) {
	key := []byte("12345678901234567890123456789012")
	service := &StorageService{encryptionKey: key}

	testCases := []struct {
		name      string
		plaintext string
	}{
		{
			name:      "Empty string",
			plaintext: "",
		},
		{
			name:      "Single character",
			plaintext: "a",
		},
		{
			name:      "Short message",
			plaintext: "Hello",
		},
		{
			name:      "Medium message",
			plaintext: "This is a test message with some content",
		},
		{
			name:      "Long message",
			plaintext: strings.Repeat("Long message content. ", 100),
		},
		{
			name:      "Unicode characters",
			plaintext: "Hello 世界 🌍 مرحبا Привет",
		},
		{
			name:      "Special characters",
			plaintext: "!@#$%^&*()_+-=[]{}|;':\",./<>?`~",
		},
		{
			name:      "Newlines and tabs",
			plaintext: "Line 1\nLine 2\tTabbed\r\nWindows line",
		},
		{
			name:      "JSON-like content",
			plaintext: `{"key": "value", "number": 123, "nested": {"data": true}}`,
		},
		{
			name:      "SQL-like content",
			plaintext: "SELECT * FROM users WHERE id = 1; DROP TABLE users;",
		},
		{
			name:      "Binary-like content",
			plaintext: "\x00\x01\x02\x03\x04\x05\x06\x07\x08\x09",
		},
		{
			name:      "Repeated patterns",
			plaintext: strings.Repeat("A", 1000),
		},
		{
			name:      "Mixed content",
			plaintext: "User: john@example.com\nPassword: P@ssw0rd123!\nSession: abc-def-ghi\nData: 你好",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Encrypt
			encrypted, err := service.encrypt(tc.plaintext)
			require.NoError(t, err, "Encryption should succeed")

			// Verify encryption happened (unless empty string)
			if tc.plaintext != "" {
				assert.NotEqual(t, tc.plaintext, encrypted, "Encrypted text should differ from plaintext")
			}

			// Decrypt
			decrypted, err := service.decrypt(encrypted)
			require.NoError(t, err, "Decryption should succeed")

			// Verify round-trip integrity
			assert.Equal(t, tc.plaintext, decrypted, "Decrypted text should match original")
		})
	}
}

// TestEncryption_NonDeterministic verifies that encryption produces different outputs
// Validates requirement 6.3: Test encryption round-trip (non-deterministic behavior)
func TestEncryption_NonDeterministic(t *testing.T) {
	key := []byte("12345678901234567890123456789012")
	service := &StorageService{encryptionKey: key}

	plaintext := "Same message encrypted multiple times"

	// Encrypt the same message 10 times
	encrypted := make([]string, 10)
	for i := 0; i < 10; i++ {
		var err error
		encrypted[i], err = service.encrypt(plaintext)
		require.NoError(t, err)
	}

	// Verify all encryptions are different (due to random nonce)
	for i := 0; i < len(encrypted); i++ {
		for j := i + 1; j < len(encrypted); j++ {
			assert.NotEqual(t, encrypted[i], encrypted[j],
				"Encryption %d and %d should produce different ciphertexts", i, j)
		}
	}

	// Verify all can be decrypted to the same plaintext
	for i, enc := range encrypted {
		decrypted, err := service.decrypt(enc)
		require.NoError(t, err, "Decryption %d should succeed", i)
		assert.Equal(t, plaintext, decrypted, "Decryption %d should match original", i)
	}
}

// TestEncryption_NoKey tests behavior when no encryption key is provided
// Validates requirement 6.3: Test encryption round-trip (no-op case)
func TestEncryption_NoKey(t *testing.T) {
	service := &StorageService{
		encryptionKey: nil,
	}

	testCases := []string{
		"",
		"Simple message",
		"Message with unicode: 你好",
		"Message with special chars: !@#$%",
	}

	for _, plaintext := range testCases {
		t.Run(plaintext, func(t *testing.T) {
			// Encrypt without key (should return plaintext)
			encrypted, err := service.encrypt(plaintext)
			require.NoError(t, err)
			assert.Equal(t, plaintext, encrypted, "Without key, encrypt should return plaintext")

			// Decrypt without key (should return input as-is)
			decrypted, err := service.decrypt(encrypted)
			require.NoError(t, err)
			assert.Equal(t, plaintext, decrypted, "Without key, decrypt should return input as-is")
		})
	}
}

// TestEncryption_EmptyKey tests behavior with empty key
func TestEncryption_EmptyKey(t *testing.T) {
	service := &StorageService{
		encryptionKey: []byte{},
	}

	plaintext := "Test message"

	// Empty key should behave like no key
	encrypted, err := service.encrypt(plaintext)
	require.NoError(t, err)
	assert.Equal(t, plaintext, encrypted, "With empty key, encrypt should return plaintext")

	decrypted, err := service.decrypt(encrypted)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted, "With empty key, decrypt should return input as-is")
}

// TestEncryption_ConcurrentOperations tests thread safety of encryption/decryption
// Validates requirement 6.3: Test encryption round-trip (concurrent access)
func TestEncryption_ConcurrentOperations(t *testing.T) {
	key := []byte("12345678901234567890123456789012")
	service := &StorageService{encryptionKey: key}

	plaintext := "Concurrent encryption test"
	iterations := 100

	// Run concurrent encryptions
	done := make(chan bool, iterations)
	for i := 0; i < iterations; i++ {
		go func() {
			encrypted, err := service.encrypt(plaintext)
			assert.NoError(t, err)

			decrypted, err := service.decrypt(encrypted)
			assert.NoError(t, err)
			assert.Equal(t, plaintext, decrypted)

			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < iterations; i++ {
		<-done
	}
}

// TestEncryption_LargeData tests encryption of large data
// Validates requirement 6.3: Test encryption round-trip (large data)
func TestEncryption_LargeData(t *testing.T) {
	key := []byte("12345678901234567890123456789012")
	service := &StorageService{encryptionKey: key}

	testCases := []struct {
		name string
		size int
	}{
		{"1KB", 1024},
		{"10KB", 10 * 1024},
		{"100KB", 100 * 1024},
		{"1MB", 1024 * 1024},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create large plaintext
			plaintext := strings.Repeat("A", tc.size)

			// Encrypt
			encrypted, err := service.encrypt(plaintext)
			require.NoError(t, err, "Encryption of %s should succeed", tc.name)

			// Decrypt
			decrypted, err := service.decrypt(encrypted)
			require.NoError(t, err, "Decryption of %s should succeed", tc.name)

			// Verify
			assert.Equal(t, plaintext, decrypted, "Decrypted %s should match original", tc.name)
			assert.Equal(t, tc.size, len(decrypted), "Decrypted size should match original")
		})
	}
}

// Helper function to create tampered ciphertext for testing
func createTamperedCiphertext(t *testing.T, service *StorageService, plaintext string) string {
	// Create valid ciphertext
	encrypted, err := service.encrypt(plaintext)
	require.NoError(t, err)

	// Decode from base64
	data, err := base64.StdEncoding.DecodeString(encrypted)
	require.NoError(t, err)

	// Tamper with the nonce (first 12 bytes for GCM)
	if len(data) > 12 {
		data[0] ^= 0xFF // Flip bits in first byte of nonce
	}

	// Re-encode
	return base64.StdEncoding.EncodeToString(data)
}

// TestEncryption_KeyRotation tests behavior when key changes
// Validates requirement 6.3: Test decryption failures (key rotation scenario)
func TestEncryption_KeyRotation(t *testing.T) {
	oldKey := []byte("12345678901234567890123456789012")
	newKey := []byte("abcdefghijklmnopqrstuvwxyz123456")

	serviceOld := &StorageService{encryptionKey: oldKey}
	serviceNew := &StorageService{encryptionKey: newKey}

	plaintext := "Message encrypted with old key"

	// Encrypt with old key
	encrypted, err := serviceOld.encrypt(plaintext)
	require.NoError(t, err)

	// Verify old key can decrypt
	decrypted, err := serviceOld.decrypt(encrypted)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)

	// Verify new key cannot decrypt (key rotation scenario)
	_, err = serviceNew.decrypt(encrypted)
	assert.Error(t, err, "New key should not be able to decrypt data encrypted with old key")
	assert.Contains(t, err.Error(), "failed to decrypt")
}

// TestEncryption_AESGCMProperties tests AES-GCM specific properties
// Validates requirement 6.3: Test encryption round-trip (AES-GCM properties)
func TestEncryption_AESGCMProperties(t *testing.T) {
	key := []byte("12345678901234567890123456789012")
	service := &StorageService{encryptionKey: key}

	plaintext := "Test AES-GCM properties"

	// Encrypt
	encrypted, err := service.encrypt(plaintext)
	require.NoError(t, err)

	// Decode to check structure
	data, err := base64.StdEncoding.DecodeString(encrypted)
	require.NoError(t, err)

	// AES-GCM nonce size should be 12 bytes
	block, err := aes.NewCipher(key)
	require.NoError(t, err)

	// GCM nonce size is 12 bytes
	expectedNonceSize := 12
	assert.GreaterOrEqual(t, len(data), expectedNonceSize, "Ciphertext should contain at least nonce")

	// GCM adds 16 bytes authentication tag
	expectedMinSize := expectedNonceSize + 16 // nonce + tag
	assert.GreaterOrEqual(t, len(data), expectedMinSize, "Ciphertext should contain nonce + tag + data")

	// Verify the nonce is at the beginning
	nonce := data[:expectedNonceSize]
	assert.Len(t, nonce, expectedNonceSize, "Nonce should be %d bytes", expectedNonceSize)

	// Verify decryption still works
	decrypted, err := service.decrypt(encrypted)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)

	_ = block // Use block to avoid unused variable error
}
