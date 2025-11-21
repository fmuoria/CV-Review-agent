package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fmuoria/CV-Review-agent/internal/models"
)

// TestExportToExcel_EnsuresXlsxExtension tests that .xlsx extension is added if missing
func TestExportToExcel_EnsuresXlsxExtension(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Test data
	results := []models.ApplicantResult{
		{
			Name: "Test Candidate",
			Rank: 1,
			Scores: models.Scores{
				ExperienceScore:      40.0,
				EducationScore:       18.0,
				DutiesScore:          17.0,
				CoverLetterScore:     8.0,
				TotalScore:           83.0,
				ExperienceReasoning:  "Good experience",
				EducationReasoning:   "Strong education",
				DutiesReasoning:      "Good fit",
				CoverLetterReasoning: "Well written",
			},
		},
	}

	jobDesc := models.JobDescription{
		Title:       "Software Engineer",
		Description: "Test job",
	}

	// Test without .xlsx extension
	outputPath := filepath.Join(tmpDir, "test_report")
	err := ExportToExcel(results, jobDesc, outputPath)
	if err != nil {
		t.Fatalf("ExportToExcel() failed: %v", err)
	}

	// Check that file was created with .xlsx extension
	expectedPath := outputPath + ".xlsx"
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("Expected file at %s but it doesn't exist", expectedPath)
	}
}

// TestExportToExcel_HandlesExistingXlsxExtension tests that existing .xlsx extension is preserved
func TestExportToExcel_HandlesExistingXlsxExtension(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Test data
	results := []models.ApplicantResult{
		{
			Name: "Test Candidate",
			Rank: 1,
			Scores: models.Scores{
				ExperienceScore: 40.0,
				TotalScore:      40.0,
			},
		},
	}

	jobDesc := models.JobDescription{
		Title: "Test Job",
	}

	// Test with .xlsx extension
	outputPath := filepath.Join(tmpDir, "test_report.xlsx")
	err := ExportToExcel(results, jobDesc, outputPath)
	if err != nil {
		t.Fatalf("ExportToExcel() failed: %v", err)
	}

	// Check that file was created at the correct path (no double .xlsx)
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Errorf("Expected file at %s but it doesn't exist", outputPath)
	}

	// Ensure no double extension
	if strings.HasSuffix(outputPath, ".xlsx.xlsx") {
		t.Error("Should not have double .xlsx extension")
	}
}

// TestExportToExcel_CleansPaths tests that paths are cleaned for cross-platform compatibility
func TestExportToExcel_CleansPaths(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	results := []models.ApplicantResult{
		{
			Name: "Test",
			Rank: 1,
			Scores: models.Scores{
				TotalScore: 50.0,
			},
		},
	}

	jobDesc := models.JobDescription{
		Title: "Test",
	}

	// Test with path that has multiple separators
	outputPath := filepath.Join(tmpDir, "reports", "test.xlsx")
	
	// Create the reports directory
	reportsDir := filepath.Join(tmpDir, "reports")
	if err := os.MkdirAll(reportsDir, 0755); err != nil {
		t.Fatalf("Failed to create reports directory: %v", err)
	}

	err := ExportToExcel(results, jobDesc, outputPath)
	if err != nil {
		t.Fatalf("ExportToExcel() failed: %v", err)
	}

	// Check that file was created
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Errorf("Expected file at %s but it doesn't exist", outputPath)
	}
}

// TestExportToExcel_EmptyResults tests export with empty results
func TestExportToExcel_EmptyResults(t *testing.T) {
	tmpDir := t.TempDir()

	results := []models.ApplicantResult{}
	jobDesc := models.JobDescription{
		Title: "Test Job",
	}

	outputPath := filepath.Join(tmpDir, "empty_report.xlsx")
	err := ExportToExcel(results, jobDesc, outputPath)
	if err != nil {
		t.Fatalf("ExportToExcel() should handle empty results: %v", err)
	}

	// Check that file was created even with no results
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Errorf("Expected file at %s but it doesn't exist", outputPath)
	}
}
