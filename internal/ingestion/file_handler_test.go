package ingestion

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewFileHandler(t *testing.T) {
	fh := NewFileHandler("test_uploads")
	if fh == nil {
		t.Fatal("Expected non-nil FileHandler")
	}

	if fh.uploadsDir != "test_uploads" {
		t.Errorf("Expected uploadsDir 'test_uploads', got '%s'", fh.uploadsDir)
	}
}

func TestSaveUploadedFile(t *testing.T) {
	// Create temporary directory for test
	tmpDir := filepath.Join(os.TempDir(), "cv_review_test")
	defer os.RemoveAll(tmpDir)

	fh := NewFileHandler(tmpDir)

	content := strings.NewReader("Test CV content")
	filename := "test_cv.txt"

	path, err := fh.SaveUploadedFile(filename, content)
	if err != nil {
		t.Fatalf("Failed to save file: %v", err)
	}

	expectedPath := filepath.Join(tmpDir, filename)
	if path != expectedPath {
		t.Errorf("Expected path %s, got %s", expectedPath, path)
	}

	// Verify file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("File was not created at %s", path)
	}

	// Verify file content
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if string(data) != "Test CV content" {
		t.Errorf("Expected content 'Test CV content', got '%s'", string(data))
	}
}

func TestLoadDocuments(t *testing.T) {
	// Create temporary directory for test
	tmpDir := filepath.Join(os.TempDir(), "cv_review_test_load")
	defer os.RemoveAll(tmpDir)

	os.MkdirAll(tmpDir, 0755)

	// Create test files
	cvContent := []byte("John Doe CV content")
	clContent := []byte("John Doe Cover Letter")

	os.WriteFile(filepath.Join(tmpDir, "JohnDoe_CV.txt"), cvContent, 0644)
	os.WriteFile(filepath.Join(tmpDir, "JohnDoe_CoverLetter.txt"), clContent, 0644)

	fh := NewFileHandler(tmpDir)
	docs, err := fh.LoadDocuments()
	if err != nil {
		t.Fatalf("Failed to load documents: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("Expected 1 document, got %d", len(docs))
	}

	doc := docs[0]
	if doc.Name != "JohnDoe" {
		t.Errorf("Expected name 'JohnDoe', got '%s'", doc.Name)
	}

	if doc.CVContent != string(cvContent) {
		t.Errorf("CV content mismatch")
	}

	if doc.CLContent != string(clContent) {
		t.Errorf("Cover letter content mismatch")
	}
}

func TestClearUploads(t *testing.T) {
	// Create temporary directory for test
	tmpDir := filepath.Join(os.TempDir(), "cv_review_test_clear")
	defer os.RemoveAll(tmpDir)

	os.MkdirAll(tmpDir, 0755)
	os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("test"), 0644)

	fh := NewFileHandler(tmpDir)
	err := fh.ClearUploads()
	if err != nil {
		t.Fatalf("Failed to clear uploads: %v", err)
	}

	// Directory should exist but be empty
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("Expected empty directory, got %d entries", len(entries))
	}
}
