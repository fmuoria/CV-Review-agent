package agent

import (
	"errors"
	"testing"
)

// TestIsRateLimitError tests the rate limit error detection
func TestIsRateLimitError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "Nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "ResourceExhausted error",
			err:      errors.New("rpc error: code = ResourceExhausted desc = Resource exhausted"),
			expected: true,
		},
		{
			name:     "HTTP 429 error",
			err:      errors.New("HTTP 429: Too Many Requests"),
			expected: true,
		},
		{
			name:     "Rate limit error",
			err:      errors.New("rate limit exceeded"),
			expected: true,
		},
		{
			name:     "Quota error",
			err:      errors.New("quota exceeded for this project"),
			expected: true,
		},
		{
			name:     "Other error",
			err:      errors.New("connection timeout"),
			expected: false,
		},
		{
			name:     "Invalid JSON error",
			err:      errors.New("failed to parse JSON"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRateLimitError(tt.err)
			if result != tt.expected {
				t.Errorf("isRateLimitError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

// TestRateLimitConstants tests that rate limit constants are set correctly
func TestRateLimitConstants(t *testing.T) {
	// Verify that the constants are set to reasonable values
	if requestDelay.Seconds() != 4 {
		t.Errorf("requestDelay = %v, want 4 seconds", requestDelay)
	}

	if maxRetries != 3 {
		t.Errorf("maxRetries = %d, want 3", maxRetries)
	}

	if retryBackoff.Seconds() != 10 {
		t.Errorf("retryBackoff = %v, want 10 seconds", retryBackoff)
	}
}
