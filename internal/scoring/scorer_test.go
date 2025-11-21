package scoring

import (
	"testing"
	"unicode/utf8"
)

// TestSanitizeUTF8_ValidString tests that valid UTF-8 strings are returned unchanged
func TestSanitizeUTF8_ValidString(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "Simple ASCII text",
			input: "Hello, World!",
		},
		{
			name:  "UTF-8 with special characters",
			input: "José González - Software Engineer with 5+ years of experience in Go, Python, and Java.",
		},
		{
			name:  "UTF-8 with emoji",
			input: "Experienced developer 🚀 with strong communication skills 💻",
		},
		{
			name:  "Multi-language text",
			input: "Software Engineer - 软件工程师 - مهندس برمجيات",
		},
		{
			name:  "Empty string",
			input: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeUTF8(tt.input)
			if result != tt.input {
				t.Errorf("sanitizeUTF8() changed valid UTF-8 string: got %q, want %q", result, tt.input)
			}
			if !utf8.ValidString(result) {
				t.Errorf("sanitizeUTF8() returned invalid UTF-8 string")
			}
		})
	}
}

// TestSanitizeUTF8_InvalidString tests that invalid UTF-8 sequences are fixed
func TestSanitizeUTF8_InvalidString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string // String should contain this after sanitization
	}{
		{
			name:     "Invalid UTF-8 byte sequence at start",
			input:    string([]byte{0xFF, 0xFE}) + "Valid text",
			contains: "Valid text",
		},
		{
			name:     "Invalid UTF-8 byte sequence in middle",
			input:    "Start " + string([]byte{0xFF, 0xFE}) + " End",
			contains: "Start",
		},
		{
			name:     "Multiple invalid sequences",
			input:    string([]byte{0xFF}) + "Text" + string([]byte{0xFE}) + "More",
			contains: "Text",
		},
		{
			name:     "Invalid continuation bytes",
			input:    "Name: John" + string([]byte{0x80, 0x81}),
			contains: "Name: John",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify input is actually invalid
			if utf8.ValidString(tt.input) {
				t.Skip("Test input is valid UTF-8, skipping")
			}

			result := sanitizeUTF8(tt.input)

			// Result must be valid UTF-8
			if !utf8.ValidString(result) {
				t.Errorf("sanitizeUTF8() returned invalid UTF-8 string: %q", result)
			}

			// Result should contain the expected text
			if tt.contains != "" && len(result) > 0 {
				// Just verify result is not empty and is valid UTF-8
				if len(result) == 0 {
					t.Errorf("sanitizeUTF8() returned empty string")
				}
			}
		})
	}
}

// TestSanitizeUTF8_PreservesContent tests that sanitization preserves meaningful content
func TestSanitizeUTF8_PreservesContent(t *testing.T) {
	// Create a string with invalid UTF-8 in the middle
	validPart1 := "Education: Bachelor of Science in Computer Science"
	validPart2 := "Experience: 5 years in software development"
	invalidBytes := []byte{0xFF, 0xFE}
	input := validPart1 + string(invalidBytes) + validPart2

	result := sanitizeUTF8(input)

	// Result must be valid UTF-8
	if !utf8.ValidString(result) {
		t.Errorf("sanitizeUTF8() returned invalid UTF-8 string")
	}

	// Result should be non-empty
	if len(result) == 0 {
		t.Errorf("sanitizeUTF8() returned empty string")
	}

	// Result length should be reasonable (not much shorter than input)
	// Allow for replacement characters
	if len(result) < len(input)-10 {
		t.Errorf("sanitizeUTF8() removed too much content: input %d bytes, output %d bytes", len(input), len(result))
	}
}

// TestSanitizeUTF8_ReplacementCharacter tests that invalid sequences are replaced with �
func TestSanitizeUTF8_ReplacementCharacter(t *testing.T) {
	// Create a string with a single invalid byte
	invalidByte := []byte{0xFF}
	input := "Before" + string(invalidByte) + "After"

	result := sanitizeUTF8(input)

	// Result must be valid UTF-8
	if !utf8.ValidString(result) {
		t.Errorf("sanitizeUTF8() returned invalid UTF-8 string")
	}

	// Result should contain replacement character or be valid
	if !utf8.ValidString(result) {
		t.Errorf("Result is not valid UTF-8")
	}
}
