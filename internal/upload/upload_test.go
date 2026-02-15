package upload

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewUploadService(t *testing.T) {
	tests := []struct {
		name        string
		site        string
		entryName   string
		statsColl   interface{} // Using interface{} to allow nil
		wantErr     bool
		errContains string
	}{
		{
			name:        "empty site",
			site:        "",
			entryName:   "uploads",
			statsColl:   nil,
			wantErr:     true,
			errContains: "site cannot be empty",
		},
		{
			name:        "empty entry name",
			site:        "CHAT",
			entryName:   "",
			statsColl:   nil,
			wantErr:     true,
			errContains: "entry name cannot be empty",
		},
		{
			name:        "nil stats collection",
			site:        "CHAT",
			entryName:   "uploads",
			statsColl:   nil,
			wantErr:     true,
			errContains: "stats collection cannot be nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var service *UploadService
			var err error
			
			if tt.statsColl == nil {
				service, err = NewUploadService(tt.site, tt.entryName, nil)
			}

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				assert.Nil(t, service)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, service)
			}
		})
	}
}

func TestUploadService_UploadFile_Validation(t *testing.T) {
	// Note: We can't create a real service without MongoDB connection
	// These tests verify the validation logic only
	
	tests := []struct {
		name        string
		file        *bytes.Buffer
		filename    string
		userID      string
		wantErr     bool
		expectedErr error
	}{
		{
			name:        "nil file",
			file:        nil,
			filename:    "test.txt",
			userID:      "user123",
			wantErr:     true,
			expectedErr: ErrInvalidFile,
		},
		{
			name:        "empty filename",
			file:        bytes.NewBufferString("test content"),
			filename:    "",
			userID:      "user123",
			wantErr:     true,
			expectedErr: ErrInvalidFilename,
		},
		{
			name:     "empty user ID",
			file:     bytes.NewBufferString("test content"),
			filename: "test.txt",
			userID:   "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock service (without actual initialization)
			service := &UploadService{
				site:      "CHAT",
				entryName: "uploads",
			}
			
			ctx := context.Background()
			
			var result *UploadResult
			var err error
			
			if tt.file != nil {
				result, err = service.UploadFile(ctx, tt.file, tt.filename, tt.userID)
			} else {
				result, err = service.UploadFile(ctx, nil, tt.filename, tt.userID)
			}

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, result)
				if tt.expectedErr != nil {
					assert.ErrorIs(t, err, tt.expectedErr)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}

func TestUploadService_GenerateSignedURL_Validation(t *testing.T) {
	// Create a mock service
	service := &UploadService{
		site:      "CHAT",
		entryName: "uploads",
	}

	tests := []struct {
		name        string
		fileID      string
		expiration  time.Duration
		wantErr     bool
		expectedErr error
	}{
		{
			name:        "empty file ID",
			fileID:      "",
			expiration:  1 * time.Hour,
			wantErr:     true,
			expectedErr: ErrInvalidFileID,
		},
		{
			name:       "valid file ID",
			fileID:     "test-file-123",
			expiration: 1 * time.Hour,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			url, err := service.GenerateSignedURL(ctx, tt.fileID, tt.expiration)

			if tt.wantErr {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedErr)
				assert.Empty(t, url)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.fileID, url)
			}
		})
	}
}

func TestUploadService_DownloadFile_Validation(t *testing.T) {
	// Create a mock service
	service := &UploadService{
		site:      "CHAT",
		entryName: "uploads",
	}

	tests := []struct {
		name        string
		filePath    string
		wantErr     bool
		expectedErr error
	}{
		{
			name:        "empty file path",
			filePath:    "",
			wantErr:     true,
			expectedErr: ErrInvalidFileID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			content, filename, err := service.DownloadFile(ctx, tt.filePath)

			if tt.wantErr {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedErr)
				assert.Nil(t, content)
				assert.Empty(t, filename)
			}
		})
	}
}

func TestUploadService_DeleteFile_Validation(t *testing.T) {
	// Create a mock service
	service := &UploadService{
		site:      "CHAT",
		entryName: "uploads",
	}

	tests := []struct {
		name        string
		fileID      string
		wantErr     bool
		expectedErr error
	}{
		{
			name:        "empty file ID",
			fileID:      "",
			wantErr:     true,
			expectedErr: ErrInvalidFileID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			err := service.DeleteFile(ctx, tt.fileID)

			if tt.wantErr {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedErr)
			}
		})
	}
}

func TestUploadResult_Structure(t *testing.T) {
	result := &UploadResult{
		FileID:   "test-file-123",
		FileURL:  "/chat-files/1225/abc12/test-file-123",
		Size:     1024,
		MimeType: "text/plain",
	}

	assert.Equal(t, "test-file-123", result.FileID)
	assert.Equal(t, "/chat-files/1225/abc12/test-file-123", result.FileURL)
	assert.Equal(t, int64(1024), result.Size)
	assert.Equal(t, "text/plain", result.MimeType)
}

func TestUploadService_ContextCancellation(t *testing.T) {
	service := &UploadService{
		site:        "CHAT",
		entryName:   "uploads",
		maxFileSize: 10 * 1024 * 1024,
	}

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Try to upload with cancelled context
	file := bytes.NewBufferString("test content")
	result, err := service.UploadFile(ctx, file, "test.txt", "user123")

	// Should fail due to validation or cancelled context
	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestUploadService_ValidateFile_Size(t *testing.T) {
	service := &UploadService{
		site:        "CHAT",
		entryName:   "uploads",
		maxFileSize: 1024, // 1KB limit for testing
	}

	tests := []struct {
		name        string
		content     string
		filename    string
		wantErr     bool
		expectedErr error
	}{
		{
			name:     "file within size limit",
			content:  "small file content",
			filename: "small.txt",
			wantErr:  false,
		},
		{
			name:        "file exceeds size limit",
			content:     strings.Repeat("x", 2000), // 2KB
			filename:    "large.txt",
			wantErr:     true,
			expectedErr: ErrFileTooLarge,
		},
		{
			name:     "file at exact size limit",
			content:  strings.Repeat("x", 1024), // Exactly 1KB
			filename: "exact.txt",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := bytes.NewBufferString(tt.content)
			content, err := service.ValidateFile(file, tt.filename)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.expectedErr != nil {
					assert.ErrorIs(t, err, tt.expectedErr)
				}
				assert.Nil(t, content)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, content)
				assert.Equal(t, len(tt.content), len(content))
			}
		})
	}
}

func TestUploadService_ValidateFile_Type(t *testing.T) {
	service := &UploadService{
		site:        "CHAT",
		entryName:   "uploads",
		maxFileSize: 10 * 1024 * 1024, // 10MB
	}

	tests := []struct {
		name        string
		content     []byte
		filename    string
		wantErr     bool
		expectedErr error
	}{
		{
			name:     "valid text file",
			content:  []byte("plain text content"),
			filename: "document.txt",
			wantErr:  false,
		},
		{
			name:     "valid JSON file",
			content:  []byte(`{"key": "value"}`),
			filename: "data.json",
			wantErr:  false,
		},
		{
			name:     "valid PNG image",
			content:  []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, // PNG signature
			filename: "image.png",
			wantErr:  false,
		},
		{
			name:     "valid JPEG image",
			content:  []byte{0xFF, 0xD8, 0xFF, 0xE0}, // JPEG signature
			filename: "photo.jpg",
			wantErr:  false,
		},
		{
			name:     "valid PDF document",
			content:  []byte("%PDF-1.4"),
			filename: "document.pdf",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := bytes.NewReader(tt.content)
			content, err := service.ValidateFile(file, tt.filename)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.expectedErr != nil {
					assert.ErrorIs(t, err, tt.expectedErr)
				}
				assert.Nil(t, content)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, content)
			}
		})
	}
}

func TestUploadService_ValidateFile_MaliciousContent(t *testing.T) {
	service := &UploadService{
		site:        "CHAT",
		entryName:   "uploads",
		maxFileSize: 10 * 1024 * 1024, // 10MB
	}

	tests := []struct {
		name        string
		content     []byte
		filename    string
		wantErr     bool
		expectedErr error
	}{
		{
			name:        "Windows executable (MZ signature)",
			content:     []byte{0x4D, 0x5A, 0x90, 0x00}, // MZ header
			filename:    "malware.exe",
			wantErr:     true,
			expectedErr: ErrMaliciousFile,
		},
		{
			name:        "ELF executable",
			content:     []byte{0x7F, 0x45, 0x4C, 0x46}, // ELF header
			filename:    "binary",
			wantErr:     true,
			expectedErr: ErrMaliciousFile,
		},
		{
			name:        "Shell script with shebang",
			content:     []byte("#!/bin/bash\nrm -rf /"),
			filename:    "script.sh",
			wantErr:     true,
			expectedErr: ErrMaliciousFile,
		},
		{
			name:        "HTML with script tag",
			content:     []byte("<html><script>alert('xss')</script></html>"),
			filename:    "page.html",
			wantErr:     true,
			expectedErr: ErrMaliciousFile,
		},
		{
			name:        "SVG with embedded script",
			content:     []byte(`<svg><script>alert('xss')</script></svg>`),
			filename:    "image.svg",
			wantErr:     true,
			expectedErr: ErrMaliciousFile,
		},
		{
			name:        "file with double extension",
			content:     []byte("fake pdf content"),
			filename:    "document.pdf.exe",
			wantErr:     true,
			expectedErr: ErrMaliciousFile,
		},
		{
			name:        "file with path traversal",
			content:     []byte("content"),
			filename:    "../../../etc/passwd",
			wantErr:     true,
			expectedErr: ErrMaliciousFile,
		},
		{
			name:        "file with null byte",
			content:     []byte("content"),
			filename:    "file\x00.txt",
			wantErr:     true,
			expectedErr: ErrMaliciousFile,
		},
		{
			name:     "safe text file",
			content:  []byte("This is safe content"),
			filename: "safe.txt",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := bytes.NewReader(tt.content)
			content, err := service.ValidateFile(file, tt.filename)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.expectedErr != nil {
					assert.ErrorIs(t, err, tt.expectedErr)
				}
				assert.Nil(t, content)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, content)
			}
		})
	}
}

func TestUploadService_SetMaxFileSize(t *testing.T) {
	service := &UploadService{
		site:        "CHAT",
		entryName:   "uploads",
		maxFileSize: 1024,
	}

	assert.Equal(t, int64(1024), service.maxFileSize)

	service.SetMaxFileSize(2048)
	assert.Equal(t, int64(2048), service.maxFileSize)
}

func TestUploadService_ValidateFile_NilFile(t *testing.T) {
	service := &UploadService{
		site:        "CHAT",
		entryName:   "uploads",
		maxFileSize: 1024,
	}

	content, err := service.ValidateFile(nil, "test.txt")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidFile)
	assert.Nil(t, content)
}

func TestUploadService_ValidateFile_EmptyFilename(t *testing.T) {
	service := &UploadService{
		site:        "CHAT",
		entryName:   "uploads",
		maxFileSize: 1024,
	}

	file := bytes.NewBufferString("content")
	content, err := service.ValidateFile(file, "")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidFilename)
	assert.Nil(t, content)
}

func TestAllowedMimeTypes(t *testing.T) {
	// Test that common MIME types are allowed
	allowedTypes := []string{
		"image/jpeg",
		"image/png",
		"application/pdf",
		"text/plain",
		"audio/mpeg",
		"video/mp4",
		"application/json",
	}

	for _, mimeType := range allowedTypes {
		assert.True(t, AllowedMimeTypes[mimeType], 
			"MIME type %s should be allowed", mimeType)
	}

	// Test that dangerous types are not allowed
	disallowedTypes := []string{
		"application/x-executable",
		"application/x-msdownload",
		"application/x-sh",
	}

	for _, mimeType := range disallowedTypes {
		assert.False(t, AllowedMimeTypes[mimeType], 
			"MIME type %s should not be allowed", mimeType)
	}
}

func TestMaliciousPatterns(t *testing.T) {
	// Verify that malicious patterns are defined
	require.NotEmpty(t, MaliciousPatterns, "Malicious patterns should be defined")

	// Check for common executable signatures
	hasExecutableSignatures := false
	for _, pattern := range MaliciousPatterns {
		if bytes.Equal(pattern, []byte{0x4D, 0x5A}) || // MZ
		   bytes.Equal(pattern, []byte{0x7F, 0x45, 0x4C, 0x46}) { // ELF
			hasExecutableSignatures = true
			break
		}
	}
	assert.True(t, hasExecutableSignatures, 
		"Malicious patterns should include executable signatures")
}
