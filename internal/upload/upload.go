package upload

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/real-rm/gomongo"
	"github.com/real-rm/goupload"
)

var (
	// ErrInvalidFile is returned when file is nil
	ErrInvalidFile = errors.New("file cannot be nil")
	// ErrInvalidFilename is returned when filename is empty
	ErrInvalidFilename = errors.New("filename cannot be empty")
	// ErrInvalidFileID is returned when file ID is empty
	ErrInvalidFileID = errors.New("file ID cannot be empty")
	// ErrFileTooLarge is returned when file exceeds size limit
	ErrFileTooLarge = errors.New("file size exceeds limit")
	// ErrInvalidFileType is returned when file type is not allowed
	ErrInvalidFileType = errors.New("file type not allowed")
	// ErrMaliciousFile is returned when file appears to be malicious
	ErrMaliciousFile = errors.New("file appears to be malicious")
)

// AllowedMimeTypes defines the whitelist of allowed file types
var AllowedMimeTypes = map[string]bool{
	// Images
	"image/jpeg":    true,
	"image/jpg":     true,
	"image/png":     true,
	"image/gif":     true,
	"image/webp":    true,
	"image/svg+xml": true,
	// Documents
	"application/pdf":    true,
	"application/msword": true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
	"application/vnd.ms-excel": true,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         true,
	"application/vnd.ms-powerpoint":                                             true,
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": true,
	"text/plain": true,
	"text/csv":   true,
	// Audio
	"audio/mpeg": true,
	"audio/mp3":  true,
	"audio/wav":  true,
	"audio/webm": true,
	"audio/ogg":  true,
	"audio/aac":  true,
	"audio/m4a":  true,
	// Video
	"video/mp4":  true,
	"video/webm": true,
	"video/ogg":  true,
	// Archives
	"application/zip":              true,
	"application/x-zip":            true,
	"application/x-zip-compressed": true,
	"application/gzip":             true,
	"application/x-gzip":           true,
	"application/x-tar":            true,
	// JSON
	"application/json": true,
}

// MaliciousPatterns contains byte patterns that indicate potentially malicious files
var MaliciousPatterns = [][]byte{
	// Executable signatures
	{0x4D, 0x5A},             // MZ (Windows executable)
	{0x7F, 0x45, 0x4C, 0x46}, // ELF (Linux executable)
	{0xCA, 0xFE, 0xBA, 0xBE}, // Mach-O (macOS executable)
	{0xFE, 0xED, 0xFA, 0xCE}, // Mach-O (macOS executable, 32-bit)
	{0xFE, 0xED, 0xFA, 0xCF}, // Mach-O (macOS executable, 64-bit)
	{0xCE, 0xFA, 0xED, 0xFE}, // Mach-O (macOS executable, reverse byte order)
	// Script signatures
	[]byte("#!/bin/sh"),
	[]byte("#!/bin/bash"),
	[]byte("#!/usr/bin/env"),
	// Potentially dangerous HTML/JS
	[]byte("<script"),
	[]byte("javascript:"),
	[]byte("onerror="),
	[]byte("onload="),
}

// UploadService manages file storage using goupload
type UploadService struct {
	statsUpdater goupload.StatsUpdater
	site         string
	entryName    string
	maxFileSize  int64 // Maximum file size in bytes
}

// UploadResult contains information about an uploaded file
type UploadResult struct {
	FileID   string
	FileURL  string
	Size     int64
	MimeType string
}

// NewUploadService creates a new upload service using goupload
func NewUploadService(site, entryName string, statsColl *gomongo.MongoCollection) (*UploadService, error) {
	if site == "" {
		return nil, errors.New("site cannot be empty")
	}

	if entryName == "" {
		return nil, errors.New("entry name cannot be empty")
	}

	if statsColl == nil {
		return nil, errors.New("stats collection cannot be nil")
	}

	// Create stats updater for file tracking
	statsUpdater, err := goupload.NewStatsUpdater(site, entryName, statsColl)
	if err != nil {
		return nil, fmt.Errorf("failed to create stats updater: %w", err)
	}

	return &UploadService{
		statsUpdater: statsUpdater,
		site:         site,
		entryName:    entryName,
		maxFileSize:  100 * 1024 * 1024, // Default 100MB
	}, nil
}

// SetMaxFileSize sets the maximum allowed file size in bytes
func (u *UploadService) SetMaxFileSize(size int64) {
	u.maxFileSize = size
}

// ValidateFile validates file size, type, and scans for malicious content
func (u *UploadService) ValidateFile(file io.Reader, filename string) ([]byte, error) {
	if file == nil {
		return nil, ErrInvalidFile
	}

	if filename == "" {
		return nil, ErrInvalidFilename
	}

	// Read file content into buffer for validation
	buf := new(bytes.Buffer)
	n, err := io.Copy(buf, file)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	content := buf.Bytes()

	// Validate file size
	if n > u.maxFileSize {
		return nil, fmt.Errorf("%w: file size %d bytes exceeds limit %d bytes",
			ErrFileTooLarge, n, u.maxFileSize)
	}

	// Scan for malicious content first (before type checking)
	if err := u.scanMaliciousContent(content, filename); err != nil {
		return nil, err
	}

	// Validate file type
	if err := u.validateFileType(content, filename); err != nil {
		return nil, err
	}

	return content, nil
}

// validateFileType checks if the file type is allowed based on MIME type
func (u *UploadService) validateFileType(content []byte, filename string) error {
	// Detect MIME type from content
	detectedMimeType := http.DetectContentType(content)

	// Strip charset and other parameters from MIME type
	baseMimeType := strings.Split(detectedMimeType, ";")[0]
	baseMimeType = strings.TrimSpace(baseMimeType)

	// Also check file extension
	ext := strings.ToLower(filepath.Ext(filename))
	extMimeType := mime.TypeByExtension(ext)
	baseExtMimeType := strings.Split(extMimeType, ";")[0]
	baseExtMimeType = strings.TrimSpace(baseExtMimeType)

	// Check if detected MIME type is allowed
	if !AllowedMimeTypes[baseMimeType] {
		// If content detection fails, try extension-based detection
		if baseExtMimeType != "" && AllowedMimeTypes[baseExtMimeType] {
			return nil
		}
		return fmt.Errorf("%w: %s (detected: %s, extension: %s)",
			ErrInvalidFileType, filename, detectedMimeType, extMimeType)
	}

	return nil
}

// scanMaliciousContent scans file content for malicious patterns
func (u *UploadService) scanMaliciousContent(content []byte, filename string) error {
	// Check for executable signatures and malicious patterns
	for _, pattern := range MaliciousPatterns {
		if bytes.Contains(content, pattern) {
			return fmt.Errorf("%w: detected malicious pattern in %s",
				ErrMaliciousFile, filename)
		}
	}

	// Additional checks for specific file types
	ext := strings.ToLower(filepath.Ext(filename))

	// Check for double extensions (e.g., file.pdf.exe)
	parts := strings.Split(filename, ".")
	if len(parts) > 2 {
		// Check if any part except the last one is a dangerous extension
		dangerousExts := []string{"exe", "bat", "cmd", "com", "scr", "vbs", "js", "jar", "sh", "bin", "app"}
		for i := 1; i < len(parts); i++ { // Start from 1 to skip the base filename
			partLower := strings.ToLower(parts[i])
			for _, dangerousExt := range dangerousExts {
				if partLower == dangerousExt {
					return fmt.Errorf("%w: suspicious double extension in %s",
						ErrMaliciousFile, filename)
				}
			}
		}
	}

	// Check for null bytes in filename (path traversal attempt)
	if strings.Contains(filename, "\x00") {
		return fmt.Errorf("%w: null byte in filename", ErrMaliciousFile)
	}

	// Check for path traversal attempts
	if strings.Contains(filename, "..") || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		return fmt.Errorf("%w: path traversal attempt in filename", ErrMaliciousFile)
	}

	// For HTML/SVG files, check for embedded scripts
	if ext == ".html" || ext == ".htm" || ext == ".svg" {
		contentLower := strings.ToLower(string(content))
		if strings.Contains(contentLower, "<script") ||
			strings.Contains(contentLower, "javascript:") ||
			strings.Contains(contentLower, "onerror=") ||
			strings.Contains(contentLower, "onload=") {
			return fmt.Errorf("%w: embedded script detected in %s",
				ErrMaliciousFile, filename)
		}
	}

	return nil
}

// UploadFile uploads a file using goupload and returns file information
func (u *UploadService) UploadFile(ctx context.Context, file io.Reader, filename string, userID string) (*UploadResult, error) {
	if file == nil {
		return nil, ErrInvalidFile
	}

	if filename == "" {
		return nil, ErrInvalidFilename
	}

	if userID == "" {
		return nil, errors.New("user ID cannot be empty")
	}

	// Validate file (size, type, malicious content)
	validatedContent, err := u.ValidateFile(file, filename)
	if err != nil {
		return nil, err
	}

	// Create a new reader from validated content
	validatedReader := bytes.NewReader(validatedContent)

	// Upload file using goupload
	result, err := goupload.Upload(
		ctx,
		u.statsUpdater,
		u.site,
		u.entryName,
		userID,
		validatedReader,
		filename,
		0, // 0 means auto-detect file size
	)
	if err != nil {
		return nil, fmt.Errorf("failed to upload file: %w", err)
	}

	return &UploadResult{
		FileID:   result.Filename, // Use generated filename as file ID
		FileURL:  result.Path,     // Full URL path
		Size:     result.Size,
		MimeType: result.MimeType,
	}, nil
}

// GenerateSignedURL returns the file path for downloading via goupload
// Note: This doesn't generate a traditional signed URL. Instead, it returns
// the file path that should be used with goupload.Download() function.
// The actual download should be handled by the application using goupload.Download().
func (u *UploadService) GenerateSignedURL(ctx context.Context, fileID string, expiration time.Duration) (string, error) {
	if fileID == "" {
		return "", ErrInvalidFileID
	}

	// With goupload, we return the file path that can be used with goupload.Download()
	// The fileID is actually the relative path in the storage system
	return fileID, nil
}

// DownloadFile downloads a file using goupload
func (u *UploadService) DownloadFile(ctx context.Context, filePath string) ([]byte, string, error) {
	if filePath == "" {
		return nil, "", ErrInvalidFileID
	}

	// Download file using goupload
	info, err := goupload.Download(ctx, u.site, u.entryName, filePath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to download file: %w", err)
	}

	return info.Content, info.Filename, nil
}

// DeleteFile deletes a file using goupload
func (u *UploadService) DeleteFile(ctx context.Context, fileID string) error {
	if fileID == "" {
		return ErrInvalidFileID
	}

	// Delete file using goupload
	result, err := goupload.Delete(ctx, u.statsUpdater, u.site, u.entryName, fileID)
	if err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	// Check if deletion was partial
	if result.IsPartialDelete {
		return fmt.Errorf("partial delete: %d locations deleted, %d failed",
			len(result.DeletedPaths), len(result.FailedPaths))
	}

	return nil
}
