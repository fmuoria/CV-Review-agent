package ingestion

import (
	"strings"
	"testing"
)

// TestIsBinaryData_PlainText tests that plain text is not detected as binary
func TestIsBinaryData_PlainText(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "Simple text",
			content: "This is a plain text CV with normal content.",
		},
		{
			name:    "Multi-line text",
			content: "John Doe\nSoftware Engineer\n5 years experience",
		},
		{
			name:    "Text with special chars",
			content: "Education: Bachelor's Degree in Computer Science\nGPA: 3.8/4.0",
		},
		{
			name:    "Empty string",
			content: "",
		},
		{
			name:    "Text with tabs and newlines",
			content: "Name:\tJohn\nTitle:\tEngineer\nYears:\t5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if IsBinaryData(tt.content) {
				t.Errorf("IsBinaryData() returned true for plain text: %q", tt.content)
			}
		})
	}
}

// TestIsBinaryData_PDF tests that PDF content is detected as binary
func TestIsBinaryData_PDF(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "PDF header v1.4",
			content: "%PDF-1.4\n%âãÏÓ\n",
		},
		{
			name:    "PDF header v1.5",
			content: "%PDF-1.5\n%ÓÔÅÔ\n1 0 obj\n",
		},
		{
			name:    "PDF header v1.7",
			content: "%PDF-1.7\n%%EOF",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !IsBinaryData(tt.content) {
				t.Errorf("IsBinaryData() returned false for PDF content")
			}
		})
	}
}

// TestIsBinaryData_ZIP tests that ZIP/DOCX content is detected as binary
func TestIsBinaryData_ZIP(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "ZIP magic number",
			content: "PK\x03\x04",
		},
		{
			name:    "DOCX file (ZIP format)",
			content: "PK\x03\x04\x14\x00\x00\x00",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !IsBinaryData(tt.content) {
				t.Errorf("IsBinaryData() returned false for ZIP/DOCX content")
			}
		})
	}
}

// TestIsBinaryData_HighNonPrintable tests binary detection with high non-printable chars
func TestIsBinaryData_HighNonPrintable(t *testing.T) {
	// Create a string with many non-printable characters (>30% of first 1000 chars)
	// This simulates corrupted or binary data
	// Non-printable means < 32 excluding \n, \r, \t
	var sb strings.Builder
	// Add 400 non-printable characters (bytes 0-31 except 9, 10, 13)
	for i := 0; i < 400; i++ {
		sb.WriteByte(0x01) // Non-printable byte
	}
	// Add 600 printable characters
	for i := 0; i < 600; i++ {
		sb.WriteString("x")
	}

	content := sb.String()

	if !IsBinaryData(content) {
		t.Errorf("IsBinaryData() returned false for content with high proportion of non-printable chars")
	}
}

// TestIsBinaryData_LowNonPrintable tests that text with few non-printable chars is not binary
func TestIsBinaryData_LowNonPrintable(t *testing.T) {
	// Create mostly normal text with a few non-printable characters
	content := "John Doe - Software Engineer\x00\nExperience: 5 years\nEducation: BS Computer Science"

	if IsBinaryData(content) {
		t.Errorf("IsBinaryData() returned true for mostly text content with few non-printable chars")
	}
}

// TestExtractText_TXT tests that TXT files return empty string (no extraction needed)
func TestExtractText_TXT(t *testing.T) {
	// For .txt files, ExtractText should return empty string (no extraction needed)
	result, err := ExtractText("test.txt")
	if err != nil {
		t.Errorf("ExtractText() returned error for .txt file: %v", err)
	}
	if result != "" {
		t.Errorf("ExtractText() should return empty string for .txt files, got %q", result)
	}
}

// TestExtractText_UnsupportedType tests that unsupported file types return error
func TestExtractText_UnsupportedType(t *testing.T) {
	tests := []string{
		"test.jpg",
		"test.png",
		"test.xlsx",
		"test.unknown",
	}

	for _, filename := range tests {
		t.Run(filename, func(t *testing.T) {
			_, err := ExtractText(filename)
			if err == nil {
				t.Errorf("ExtractText() should return error for unsupported file type %s", filename)
			}
			if !strings.Contains(err.Error(), "unsupported file type") {
				t.Errorf("Error message should mention 'unsupported file type', got: %v", err)
			}
		})
	}
}

// TestExtractText_DOCX tests that DOCX extraction attempts to process file
func TestExtractText_DOCX(t *testing.T) {
	// DOCX extraction will fail for non-existent files, but should not return "not implemented" error
	_, err := ExtractText("test.docx")
	if err == nil {
		t.Error("ExtractText() should return error for non-existent .docx file")
	}
	// Should fail with "failed to open" or similar, not "not implemented"
	if strings.Contains(err.Error(), "not yet implemented") {
		t.Errorf("Error should not mention 'not implemented' anymore, got: %v", err)
	}
}

// TestMin tests the min helper function
func TestMin(t *testing.T) {
	tests := []struct {
		name string
		a    int
		b    int
		want int
	}{
		{
			name: "a less than b",
			a:    5,
			b:    10,
			want: 5,
		},
		{
			name: "b less than a",
			a:    10,
			b:    5,
			want: 5,
		},
		{
			name: "equal values",
			a:    7,
			b:    7,
			want: 7,
		},
		{
			name: "negative values",
			a:    -5,
			b:    -10,
			want: -10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := min(tt.a, tt.b)
			if result != tt.want {
				t.Errorf("min(%d, %d) = %d, want %d", tt.a, tt.b, result, tt.want)
			}
		})
	}
}
