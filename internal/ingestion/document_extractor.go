package ingestion

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	// MinExtractedTextLength is the minimum text length required for successful extraction
	MinExtractedTextLength = 50
	// BinarySampleSize is the number of bytes to sample for binary detection
	BinarySampleSize = 1000
	// BinaryThreshold is the proportion of non-printable characters that indicates binary data
	BinaryThreshold = 0.3
)

// ExtractText extracts text from PDF, DOCX, DOC, or TXT files
func ExtractText(filePath string) (string, error) {
	ext := strings.ToLower(filepath.Ext(filePath))

	switch ext {
	case ".txt":
		// Plain text - no extraction needed
		return "", nil
	case ".pdf":
		return extractPDF(filePath)
	case ".docx", ".doc":
		return extractDOCX(filePath)
	default:
		return "", fmt.Errorf("unsupported file type: %s", ext)
	}
}

// extractPDF extracts text from PDF using pdftotext (if available) or returns error
func extractPDF(filePath string) (string, error) {
	// Check if pdftotext is available
	cmd := exec.Command("pdftotext", "-layout", filePath, "-")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("PDF extraction requires 'pdftotext' (install poppler-utils): %w\nFile appears to be binary PDF: %s", err, filePath)
	}

	text := string(output)
	if len(text) < MinExtractedTextLength {
		return "", fmt.Errorf("extracted text is too short (likely failed extraction) from: %s", filePath)
	}

	return text, nil
}

// extractDOCX extracts text from DOCX using antiword (for .doc) or requires manual conversion
func extractDOCX(filePath string) (string, error) {
	ext := strings.ToLower(filepath.Ext(filePath))

	if ext == ".doc" {
		// Try antiword for .doc files
		cmd := exec.Command("antiword", filePath)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("DOC extraction requires 'antiword': %w\nFile appears to be binary DOC: %s", err, filePath)
		}
		return string(output), nil
	}

	// For .docx, we'd need a Go library - for now return error with helpful message
	return "", fmt.Errorf("DOCX extraction not yet implemented. Please convert to PDF or TXT first: %s", filePath)
}

// IsBinaryData checks if content appears to be binary (PDF/ZIP markers)
func IsBinaryData(content string) bool {
	if len(content) == 0 {
		return false
	}

	// Check for PDF magic number
	if strings.HasPrefix(content, "%PDF-") {
		return true
	}

	// Check for ZIP magic number (DOCX files)
	if len(content) >= 2 && content[:2] == "PK" {
		return true
	}

	// Check for high proportion of non-printable characters
	sampleSize := min(BinarySampleSize, len(content))
	nonPrintable := 0
	for i := 0; i < sampleSize; i++ {
		ch := content[i]
		if ch < 32 && ch != '\n' && ch != '\r' && ch != '\t' {
			nonPrintable++
		}
	}

	return float64(nonPrintable)/float64(sampleSize) > BinaryThreshold
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
