package ingestion

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/fmuoria/CV-Review-agent/internal/models"
)

// FileHandler manages file operations for CV and cover letter ingestion
type FileHandler struct {
	uploadsDir string
}

// NewFileHandler creates a new file handler
func NewFileHandler(uploadsDir string) *FileHandler {
	return &FileHandler{
		uploadsDir: uploadsDir,
	}
}

// SaveUploadedFile saves an uploaded file to the uploads directory
func (fh *FileHandler) SaveUploadedFile(filename string, content io.Reader) (string, error) {
	// Ensure uploads directory exists
	if err := os.MkdirAll(fh.uploadsDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create uploads directory: %w", err)
	}

	filePath := filepath.Join(fh.uploadsDir, filename)
	file, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(file, content); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return filePath, nil
}

// LoadDocuments loads all documents from the uploads directory
func (fh *FileHandler) LoadDocuments() ([]models.ApplicantDocument, error) {
	files, err := os.ReadDir(fh.uploadsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []models.ApplicantDocument{}, nil
		}
		return nil, fmt.Errorf("failed to read uploads directory: %w", err)
	}

	// Group files by applicant name
	applicantFiles := make(map[string]*models.ApplicantDocument)

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		filename := file.Name()
		ext := strings.ToLower(filepath.Ext(filename))

		// Only process PDF and TXT files
		if ext != ".pdf" && ext != ".txt" && ext != ".doc" && ext != ".docx" {
			continue
		}

		// Extract applicant name from filename
		// Convention: "Name_CV.pdf" or "Name_CoverLetter.pdf"
		baseName := strings.TrimSuffix(filename, ext)
		parts := strings.Split(baseName, "_")

		if len(parts) < 2 {
			continue
		}

		applicantName := parts[0]
		docType := strings.ToLower(strings.Join(parts[1:], "_"))

		if applicantFiles[applicantName] == nil {
			applicantFiles[applicantName] = &models.ApplicantDocument{
				Name: applicantName,
			}
		}

		filePath := filepath.Join(fh.uploadsDir, filename)
		content, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", filename, err)
		}

		// Determine if it's a CV or cover letter
		if strings.Contains(docType, "cv") || strings.Contains(docType, "resume") {
			applicantFiles[applicantName].CVContent = string(content)
			applicantFiles[applicantName].CVPath = filePath
		} else if strings.Contains(docType, "cover") || strings.Contains(docType, "letter") || strings.Contains(docType, "cl") {
			applicantFiles[applicantName].CLContent = string(content)
			applicantFiles[applicantName].CLPath = filePath
		}
	}

	// Convert map to slice
	documents := make([]models.ApplicantDocument, 0, len(applicantFiles))
	for _, doc := range applicantFiles {
		if doc.CVContent != "" { // Only include applicants with at least a CV
			documents = append(documents, *doc)
		}
	}

	return documents, nil
}

// ClearUploads removes all files from the uploads directory
func (fh *FileHandler) ClearUploads() error {
	if err := os.RemoveAll(fh.uploadsDir); err != nil {
		return fmt.Errorf("failed to clear uploads directory: %w", err)
	}
	return os.MkdirAll(fh.uploadsDir, 0755)
}
