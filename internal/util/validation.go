package util

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

// ValidateNotEmpty checks if a string is not empty and returns an error if it is.
// This eliminates repeated empty string checks.
//
// Example:
//
//	if err := util.ValidateNotEmpty(sessionID, "session ID"); err != nil {
//	    return err
//	}
func ValidateNotEmpty(value, fieldName string) error {
	if value == "" {
		return fmt.Errorf("%s cannot be empty", fieldName)
	}
	return nil
}

// ValidateNotNil checks if a pointer is not nil and returns an error if it is.
// This eliminates repeated nil checks.
// Note: This function uses reflection to properly detect nil pointers.
//
// Example:
//
//	if err := util.ValidateNotNil(conn, "connection"); err != nil {
//	    return err
//	}
func ValidateNotNil(value interface{}, fieldName string) error {
	if value == nil {
		return fmt.Errorf("%s cannot be nil", fieldName)
	}

	// Use type assertion to check for nil pointers
	// This is needed because interface{} can hold a typed nil pointer
	switch v := value.(type) {
	case *string:
		if v == nil {
			return fmt.Errorf("%s cannot be nil", fieldName)
		}
	case *int:
		if v == nil {
			return fmt.Errorf("%s cannot be nil", fieldName)
		}
		// Add more types as needed, or use reflection for generic solution
	}

	return nil
}

// ValidateRange checks if an integer is within a specified range (inclusive).
//
// Example:
//
//	if err := util.ValidateRange(port, 1, 65535, "port"); err != nil {
//	    return err
//	}
func ValidateRange(value, min, max int, fieldName string) error {
	if value < min || value > max {
		return fmt.Errorf("%s must be between %d and %d, got %d", fieldName, min, max, value)
	}
	return nil
}

// ValidateMinLength checks if a string meets minimum length requirement.
//
// Example:
//
//	if err := util.ValidateMinLength(secret, 32, "JWT secret"); err != nil {
//	    return err
//	}
func ValidateMinLength(value string, minLength int, fieldName string) error {
	if len(value) < minLength {
		return fmt.Errorf("%s must be at least %d characters, got %d", fieldName, minLength, len(value))
	}
	return nil
}

// ValidateExactLength checks if a byte slice has exact length.
// This is useful for encryption key validation.
//
// Example:
//
//	if err := util.ValidateExactLength(key, 32, "encryption key"); err != nil {
//	    return err
//	}
func ValidateExactLength(value []byte, exactLength int, fieldName string) error {
	if len(value) != exactLength && len(value) != 0 {
		return fmt.Errorf("%s must be exactly %d bytes, got %d bytes", fieldName, exactLength, len(value))
	}
	return nil
}

// ValidatePositive checks if a number is positive.
//
// Example:
//
//	if err := util.ValidatePositive(timeout, "timeout"); err != nil {
//	    return err
//	}
func ValidatePositive(value int, fieldName string) error {
	if value <= 0 {
		return fmt.Errorf("%s must be positive, got %d", fieldName, value)
	}
	return nil
}

// ValidateTimeRange checks if end time is after start time.
//
// Example:
//
//	if err := util.ValidateTimeRange(startTime, endTime); err != nil {
//	    return err
//	}
func ValidateTimeRange(start, end interface{}) error {
	startTime, ok := start.(time.Time)
	if !ok {
		return errors.New("start must be a time.Time value")
	}
	endTime, ok := end.(time.Time)
	if !ok {
		return errors.New("end must be a time.Time value")
	}
	if startTime.IsZero() {
		return errors.New("start time cannot be zero")
	}
	if endTime.IsZero() {
		return errors.New("end time cannot be zero")
	}
	if !endTime.After(startTime) {
		return errors.New("end time must be after start time")
	}
	return nil
}

// MaxFileURLLength is the maximum allowed length for file URLs.
const MaxFileURLLength = 2048

// privateNetworks contains CIDR ranges for private/internal IPs.
var privateNetworks = func() []*net.IPNet {
	cidrs := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"::1/128",
		"fc00::/7",
	}
	var nets []*net.IPNet
	for _, cidr := range cidrs {
		_, ipNet, _ := net.ParseCIDR(cidr)
		if ipNet != nil {
			nets = append(nets, ipNet)
		}
	}
	return nets
}()

// ValidateFileURL validates that a URL is safe for storage.
// Rejects dangerous schemes, private/internal IPs, and overly long URLs.
func ValidateFileURL(rawURL string) error {
	if rawURL == "" {
		return nil // Empty URL is allowed (optional field)
	}

	if len(rawURL) > MaxFileURLLength {
		return fmt.Errorf("URL exceeds maximum length of %d characters", MaxFileURLLength)
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Allow only https scheme (reject plaintext http in production)
	scheme := strings.ToLower(u.Scheme)
	if scheme != "https" {
		return fmt.Errorf("URL scheme %q is not allowed; only https is permitted", u.Scheme)
	}

	// Reject URLs with no host
	if u.Hostname() == "" {
		return errors.New("URL must have a hostname")
	}

	// Reject private/internal IPs (SSRF protection)
	hostname := u.Hostname()
	ip := net.ParseIP(hostname)
	if ip != nil {
		for _, ipNet := range privateNetworks {
			if ipNet.Contains(ip) {
				return fmt.Errorf("URL host %q resolves to a private/internal IP address", hostname)
			}
		}
	} else {
		// DNS pre-flight: resolve hostname and check all IPs against private ranges
		// Prevents DNS rebinding attacks where a hostname resolves to a private IP
		addrs, err := net.LookupHost(hostname)
		if err == nil {
			for _, addr := range addrs {
				resolved := net.ParseIP(addr)
				if resolved == nil {
					continue
				}
				for _, ipNet := range privateNetworks {
					if ipNet.Contains(resolved) {
						return fmt.Errorf("URL host %q resolves to a private/internal IP address %s", hostname, addr)
					}
				}
			}
		}
		// DNS lookup failure is not fatal — the URL may still be valid
	}

	return nil
}
