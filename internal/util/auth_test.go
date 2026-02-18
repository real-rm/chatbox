package util

import (
	"errors"
	"testing"
)

func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		name       string
		authHeader string
		wantToken  string
		wantErr    error
	}{
		{
			name:       "valid bearer token",
			authHeader: "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
			wantToken:  "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
			wantErr:    nil,
		},
		{
			name:       "empty header",
			authHeader: "",
			wantToken:  "",
			wantErr:    ErrMissingAuthHeader,
		},
		{
			name:       "missing Bearer prefix",
			authHeader: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
			wantToken:  "",
			wantErr:    ErrInvalidAuthHeader,
		},
		{
			name:       "wrong prefix",
			authHeader: "Basic dXNlcjpwYXNz",
			wantToken:  "",
			wantErr:    ErrInvalidAuthHeader,
		},
		{
			name:       "Bearer with no token",
			authHeader: "Bearer ",
			wantToken:  "",
			wantErr:    ErrInvalidAuthHeader,
		},
		{
			name:       "Bearer with whitespace token",
			authHeader: "Bearer    ",
			wantToken:  "   ",
			wantErr:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := ExtractBearerToken(tt.authHeader)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("ExtractBearerToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if token != tt.wantToken {
				t.Errorf("ExtractBearerToken() token = %v, want %v", token, tt.wantToken)
			}
		})
	}
}

func TestHasRole(t *testing.T) {
	tests := []struct {
		name          string
		userRoles     []string
		requiredRoles []string
		want          bool
	}{
		{
			name:          "has single required role",
			userRoles:     []string{"admin", "user"},
			requiredRoles: []string{"admin"},
			want:          true,
		},
		{
			name:          "has one of multiple required roles",
			userRoles:     []string{"user", "moderator"},
			requiredRoles: []string{"admin", "moderator"},
			want:          true,
		},
		{
			name:          "has none of required roles",
			userRoles:     []string{"user"},
			requiredRoles: []string{"admin", "moderator"},
			want:          false,
		},
		{
			name:          "empty user roles",
			userRoles:     []string{},
			requiredRoles: []string{"admin"},
			want:          false,
		},
		{
			name:          "empty required roles",
			userRoles:     []string{"admin"},
			requiredRoles: []string{},
			want:          false,
		},
		{
			name:          "both empty",
			userRoles:     []string{},
			requiredRoles: []string{},
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasRole(tt.userRoles, tt.requiredRoles...); got != tt.want {
				t.Errorf("HasRole() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContainsWeakPattern(t *testing.T) {
	weakPatterns := []string{"password", "test", "admin", "12345"}

	tests := []struct {
		name         string
		input        string
		wantContains bool
		wantPattern  string
	}{
		{
			name:         "contains password",
			input:        "mypassword123",
			wantContains: true,
			wantPattern:  "password",
		},
		{
			name:         "contains test",
			input:        "testing123",
			wantContains: true,
			wantPattern:  "test",
		},
		{
			name:         "contains admin",
			input:        "admin_user",
			wantContains: true,
			wantPattern:  "admin",
		},
		{
			name:         "contains 12345",
			input:        "secret12345",
			wantContains: true,
			wantPattern:  "12345",
		},
		{
			name:         "no weak pattern",
			input:        "strong_random_secret_xyz",
			wantContains: false,
			wantPattern:  "",
		},
		{
			name:         "case insensitive",
			input:        "MyPASSWORD",
			wantContains: true,
			wantPattern:  "password",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contains, pattern := ContainsWeakPattern(tt.input, weakPatterns)
			if contains != tt.wantContains {
				t.Errorf("ContainsWeakPattern() contains = %v, want %v", contains, tt.wantContains)
			}
			if pattern != tt.wantPattern {
				t.Errorf("ContainsWeakPattern() pattern = %v, want %v", pattern, tt.wantPattern)
			}
		})
	}
}
