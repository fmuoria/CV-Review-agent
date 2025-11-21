package export

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fmuoria/CV-Review-agent/internal/models"
	"github.com/xuri/excelize/v2"
)

// ExportToExcel generates an Excel file with CV review results
func ExportToExcel(results []models.ApplicantResult, jobDesc models.JobDescription, outputPath string) error {
	f := excelize.NewFile()
	defer f.Close()

	// Ensure output path has .xlsx extension
	if !strings.HasSuffix(strings.ToLower(outputPath), ".xlsx") {
		outputPath = outputPath + ".xlsx"
	}

	// Clean the path for cross-platform compatibility (Windows paths)
	outputPath = filepath.Clean(outputPath)

	// Create sheets
	summarySheet := "Summary"
	candidatesSheet := "Ranked Candidates"
	detailsSheet := "Detailed Analysis"

	f.SetSheetName("Sheet1", summarySheet)
	f.NewSheet(candidatesSheet)
	f.NewSheet(detailsSheet)

	// Create summary sheet
	if err := createSummarySheet(f, summarySheet, results, jobDesc); err != nil {
		return fmt.Errorf("failed to create summary sheet: %w", err)
	}

	// Create ranked candidates sheet
	if err := createRankedCandidatesSheet(f, candidatesSheet, results); err != nil {
		return fmt.Errorf("failed to create ranked candidates sheet: %w", err)
	}

	// Create detailed analysis sheet
	if err := createDetailedAnalysisSheet(f, detailsSheet, results); err != nil {
		return fmt.Errorf("failed to create detailed analysis sheet: %w", err)
	}

	// Try to save the file directly
	if err := f.SaveAs(outputPath); err != nil {
		// If direct save fails, try buffer write fallback
		var buf bytes.Buffer
		if writeErr := f.Write(&buf); writeErr != nil {
			return fmt.Errorf("failed to save Excel file: direct save failed (%v), buffer write also failed: %w", err, writeErr)
		}

		// Write buffer to file
		if fileErr := os.WriteFile(outputPath, buf.Bytes(), 0644); fileErr != nil {
			return fmt.Errorf("failed to save Excel file: direct save failed (%v), file write failed: %w", err, fileErr)
		}
	}

	return nil
}

// createSummarySheet creates the summary sheet with job details and statistics
func createSummarySheet(f *excelize.File, sheetName string, results []models.ApplicantResult, jobDesc models.JobDescription) error {
	// Set column widths
	f.SetColWidth(sheetName, "A", "A", 25)
	f.SetColWidth(sheetName, "B", "B", 50)

	// Create header style
	headerStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 14, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"4472C4"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "left", Vertical: "center"},
	})
	if err != nil {
		return err
	}

	// Create label style
	labelStyle, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
	})
	if err != nil {
		return err
	}

	row := 1

	// Title
	f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), "CV Review Report")
	f.SetCellStyle(sheetName, fmt.Sprintf("A%d", row), fmt.Sprintf("B%d", row), headerStyle)
	f.MergeCell(sheetName, fmt.Sprintf("A%d", row), fmt.Sprintf("B%d", row))
	row += 2

	// Job Details
	f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), "Job Title:")
	f.SetCellStyle(sheetName, fmt.Sprintf("A%d", row), fmt.Sprintf("A%d", row), labelStyle)
	f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), jobDesc.Title)
	row++

	f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), "Generated:")
	f.SetCellStyle(sheetName, fmt.Sprintf("A%d", row), fmt.Sprintf("A%d", row), labelStyle)
	f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), time.Now().Format("2006-01-02 15:04:05"))
	row++

	f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), "Total Candidates Scored:")
	f.SetCellStyle(sheetName, fmt.Sprintf("A%d", row), fmt.Sprintf("A%d", row), labelStyle)
	f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), len(results))
	row++

	// Note about candidate count
	f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), "Note:")
	f.SetCellStyle(sheetName, fmt.Sprintf("A%d", row), fmt.Sprintf("A%d", row), labelStyle)
	noteText := "If fewer candidates than emails/files, files may have been skipped due to: " +
		"scanned images (no text), certificate-only PDFs, duplicates, unsupported formats, " +
		"or naming conventions not matching expected pattern (Name_CV.pdf / Name_CoverLetter.pdf)."
	f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), noteText)
	row += 2

	// Statistics
	f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), "Statistics:")
	f.SetCellStyle(sheetName, fmt.Sprintf("A%d", row), fmt.Sprintf("B%d", row), headerStyle)
	f.MergeCell(sheetName, fmt.Sprintf("A%d", row), fmt.Sprintf("B%d", row))
	row++

	if len(results) > 0 {
		excellent := 0
		good := 0
		fair := 0
		poor := 0

		for _, r := range results {
			score := r.Scores.TotalScore
			if score >= 90 {
				excellent++
			} else if score >= 70 {
				good++
			} else if score >= 50 {
				fair++
			} else {
				poor++
			}
		}

		f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), "Excellent (90-100):")
		f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), excellent)
		row++

		f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), "Good (70-89):")
		f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), good)
		row++

		f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), "Fair (50-69):")
		f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), fair)
		row++

		f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), "Poor (<50):")
		f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), poor)
		row += 2

		// Average score
		var totalScore float64
		for _, r := range results {
			totalScore += r.Scores.TotalScore
		}
		avgScore := totalScore / float64(len(results))

		f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), "Average Score:")
		f.SetCellStyle(sheetName, fmt.Sprintf("A%d", row), fmt.Sprintf("A%d", row), labelStyle)
		f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), fmt.Sprintf("%.2f", avgScore))
		row += 2

		// Additional detailed statistics
		f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), "Score Distribution Details:")
		f.SetCellStyle(sheetName, fmt.Sprintf("A%d", row), fmt.Sprintf("B%d", row), headerStyle)
		f.MergeCell(sheetName, fmt.Sprintf("A%d", row), fmt.Sprintf("B%d", row))
		row++

		// Calculate min, max, median
		if len(results) > 0 {
			var minScore, maxScore float64
			minScore = results[0].Scores.TotalScore
			maxScore = results[0].Scores.TotalScore

			for _, r := range results {
				if r.Scores.TotalScore < minScore {
					minScore = r.Scores.TotalScore
				}
				if r.Scores.TotalScore > maxScore {
					maxScore = r.Scores.TotalScore
				}
			}

			f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), "Highest Score:")
			f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), fmt.Sprintf("%.2f", maxScore))
			row++

			f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), "Lowest Score:")
			f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), fmt.Sprintf("%.2f", minScore))
			row++

			f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), "Score Range:")
			f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), fmt.Sprintf("%.2f", maxScore-minScore))
			row += 2

			// Count candidates with/without cover letters
			withCL := 0
			for _, r := range results {
				if r.CLPath != "" {
					withCL++
				}
			}

			f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), "Candidates with Cover Letter:")
			f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), withCL)
			row++

			f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), "Candidates without Cover Letter:")
			f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), len(results)-withCL)
			row++
		}
	}

	return nil
}

// createRankedCandidatesSheet creates the ranked candidates sheet with color-coding
func createRankedCandidatesSheet(f *excelize.File, sheetName string, results []models.ApplicantResult) error {
	// Set column widths
	f.SetColWidth(sheetName, "A", "A", 8)
	f.SetColWidth(sheetName, "B", "B", 25)
	f.SetColWidth(sheetName, "C", "C", 15)
	f.SetColWidth(sheetName, "D", "D", 15)
	f.SetColWidth(sheetName, "E", "E", 15)
	f.SetColWidth(sheetName, "F", "F", 15)
	f.SetColWidth(sheetName, "G", "G", 15)
	f.SetColWidth(sheetName, "H", "H", 12)
	f.SetColWidth(sheetName, "I", "I", 12)

	// Create header style
	headerStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"4472C4"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border: []excelize.Border{
			{Type: "left", Color: "000000", Style: 1},
			{Type: "right", Color: "000000", Style: 1},
			{Type: "top", Color: "000000", Style: 1},
			{Type: "bottom", Color: "000000", Style: 1},
		},
	})
	if err != nil {
		return err
	}

	// Create row styles with color-coding
	excellentStyle, _ := f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Color: []string{"C6EFCE"}, Pattern: 1},
		Border: []excelize.Border{
			{Type: "left", Color: "000000", Style: 1},
			{Type: "right", Color: "000000", Style: 1},
			{Type: "top", Color: "000000", Style: 1},
			{Type: "bottom", Color: "000000", Style: 1},
		},
	})

	goodStyle, _ := f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Color: []string{"FFEB9C"}, Pattern: 1},
		Border: []excelize.Border{
			{Type: "left", Color: "000000", Style: 1},
			{Type: "right", Color: "000000", Style: 1},
			{Type: "top", Color: "000000", Style: 1},
			{Type: "bottom", Color: "000000", Style: 1},
		},
	})

	fairStyle, _ := f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Color: []string{"FFC7CE"}, Pattern: 1},
		Border: []excelize.Border{
			{Type: "left", Color: "000000", Style: 1},
			{Type: "right", Color: "000000", Style: 1},
			{Type: "top", Color: "000000", Style: 1},
			{Type: "bottom", Color: "000000", Style: 1},
		},
	})

	poorStyle, _ := f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Color: []string{"FF9999"}, Pattern: 1},
		Border: []excelize.Border{
			{Type: "left", Color: "000000", Style: 1},
			{Type: "right", Color: "000000", Style: 1},
			{Type: "top", Color: "000000", Style: 1},
			{Type: "bottom", Color: "000000", Style: 1},
		},
	})

	// Set headers
	headers := []string{"Rank", "Candidate", "Total Score", "Experience", "Education", "Duties", "Cover Letter", "CV Link", "CL Link"}
	for col, header := range headers {
		cell := fmt.Sprintf("%s1", string(rune('A'+col)))
		f.SetCellValue(sheetName, cell, header)
		f.SetCellStyle(sheetName, cell, cell, headerStyle)
	}

	// Populate data
	for i, result := range results {
		row := i + 2
		f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), result.Rank)
		f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), result.Name)
		f.SetCellValue(sheetName, fmt.Sprintf("C%d", row), fmt.Sprintf("%.2f", result.Scores.TotalScore))
		f.SetCellValue(sheetName, fmt.Sprintf("D%d", row), fmt.Sprintf("%.2f", result.Scores.ExperienceScore))
		f.SetCellValue(sheetName, fmt.Sprintf("E%d", row), fmt.Sprintf("%.2f", result.Scores.EducationScore))
		f.SetCellValue(sheetName, fmt.Sprintf("F%d", row), fmt.Sprintf("%.2f", result.Scores.DutiesScore))
		f.SetCellValue(sheetName, fmt.Sprintf("G%d", row), fmt.Sprintf("%.2f", result.Scores.CoverLetterScore))

		// Apply color-coding based on total score
		var style int
		score := result.Scores.TotalScore
		if score >= 90 {
			style = excellentStyle
		} else if score >= 70 {
			style = goodStyle
		} else if score >= 50 {
			style = fairStyle
		} else {
			style = poorStyle
		}

		f.SetCellStyle(sheetName, fmt.Sprintf("A%d", row), fmt.Sprintf("G%d", row), style)

		// Add CV Link (Column H)
		if result.CVPath != "" {
			cvCell := fmt.Sprintf("H%d", row)
			// Convert to absolute path if needed
			absPath, err := filepath.Abs(result.CVPath)
			if err != nil {
				absPath = result.CVPath
			}
			f.SetCellValue(sheetName, cvCell, "Open CV")
			// Use file:// protocol with forward slashes
			fileURL := "file:///" + strings.ReplaceAll(absPath, "\\", "/")
			f.SetCellHyperLink(sheetName, cvCell, fileURL, "External")
			// Apply link style with same background color
			if score >= 90 {
				linkStyleWithBg, _ := f.NewStyle(&excelize.Style{
					Font: &excelize.Font{Color: "0563C1", Underline: "single"},
					Fill: excelize.Fill{Type: "pattern", Color: []string{"C6EFCE"}, Pattern: 1},
					Border: []excelize.Border{
						{Type: "left", Color: "000000", Style: 1},
						{Type: "right", Color: "000000", Style: 1},
						{Type: "top", Color: "000000", Style: 1},
						{Type: "bottom", Color: "000000", Style: 1},
					},
				})
				f.SetCellStyle(sheetName, cvCell, cvCell, linkStyleWithBg)
			} else if score >= 70 {
				linkStyleWithBg, _ := f.NewStyle(&excelize.Style{
					Font: &excelize.Font{Color: "0563C1", Underline: "single"},
					Fill: excelize.Fill{Type: "pattern", Color: []string{"FFEB9C"}, Pattern: 1},
					Border: []excelize.Border{
						{Type: "left", Color: "000000", Style: 1},
						{Type: "right", Color: "000000", Style: 1},
						{Type: "top", Color: "000000", Style: 1},
						{Type: "bottom", Color: "000000", Style: 1},
					},
				})
				f.SetCellStyle(sheetName, cvCell, cvCell, linkStyleWithBg)
			} else if score >= 50 {
				linkStyleWithBg, _ := f.NewStyle(&excelize.Style{
					Font: &excelize.Font{Color: "0563C1", Underline: "single"},
					Fill: excelize.Fill{Type: "pattern", Color: []string{"FFC7CE"}, Pattern: 1},
					Border: []excelize.Border{
						{Type: "left", Color: "000000", Style: 1},
						{Type: "right", Color: "000000", Style: 1},
						{Type: "top", Color: "000000", Style: 1},
						{Type: "bottom", Color: "000000", Style: 1},
					},
				})
				f.SetCellStyle(sheetName, cvCell, cvCell, linkStyleWithBg)
			} else {
				linkStyleWithBg, _ := f.NewStyle(&excelize.Style{
					Font: &excelize.Font{Color: "0563C1", Underline: "single"},
					Fill: excelize.Fill{Type: "pattern", Color: []string{"FF9999"}, Pattern: 1},
					Border: []excelize.Border{
						{Type: "left", Color: "000000", Style: 1},
						{Type: "right", Color: "000000", Style: 1},
						{Type: "top", Color: "000000", Style: 1},
						{Type: "bottom", Color: "000000", Style: 1},
					},
				})
				f.SetCellStyle(sheetName, cvCell, cvCell, linkStyleWithBg)
			}
		} else {
			// Apply the same background style even if no link
			f.SetCellValue(sheetName, fmt.Sprintf("H%d", row), "")
			f.SetCellStyle(sheetName, fmt.Sprintf("H%d", row), fmt.Sprintf("H%d", row), style)
		}

		// Add CL Link (Column I)
		if result.CLPath != "" {
			clCell := fmt.Sprintf("I%d", row)
			absPath, err := filepath.Abs(result.CLPath)
			if err != nil {
				absPath = result.CLPath
			}
			f.SetCellValue(sheetName, clCell, "Open CL")
			fileURL := "file:///" + strings.ReplaceAll(absPath, "\\", "/")
			f.SetCellHyperLink(sheetName, clCell, fileURL, "External")
			// Apply link style with same background color
			if score >= 90 {
				linkStyleWithBg, _ := f.NewStyle(&excelize.Style{
					Font: &excelize.Font{Color: "0563C1", Underline: "single"},
					Fill: excelize.Fill{Type: "pattern", Color: []string{"C6EFCE"}, Pattern: 1},
					Border: []excelize.Border{
						{Type: "left", Color: "000000", Style: 1},
						{Type: "right", Color: "000000", Style: 1},
						{Type: "top", Color: "000000", Style: 1},
						{Type: "bottom", Color: "000000", Style: 1},
					},
				})
				f.SetCellStyle(sheetName, clCell, clCell, linkStyleWithBg)
			} else if score >= 70 {
				linkStyleWithBg, _ := f.NewStyle(&excelize.Style{
					Font: &excelize.Font{Color: "0563C1", Underline: "single"},
					Fill: excelize.Fill{Type: "pattern", Color: []string{"FFEB9C"}, Pattern: 1},
					Border: []excelize.Border{
						{Type: "left", Color: "000000", Style: 1},
						{Type: "right", Color: "000000", Style: 1},
						{Type: "top", Color: "000000", Style: 1},
						{Type: "bottom", Color: "000000", Style: 1},
					},
				})
				f.SetCellStyle(sheetName, clCell, clCell, linkStyleWithBg)
			} else if score >= 50 {
				linkStyleWithBg, _ := f.NewStyle(&excelize.Style{
					Font: &excelize.Font{Color: "0563C1", Underline: "single"},
					Fill: excelize.Fill{Type: "pattern", Color: []string{"FFC7CE"}, Pattern: 1},
					Border: []excelize.Border{
						{Type: "left", Color: "000000", Style: 1},
						{Type: "right", Color: "000000", Style: 1},
						{Type: "top", Color: "000000", Style: 1},
						{Type: "bottom", Color: "000000", Style: 1},
					},
				})
				f.SetCellStyle(sheetName, clCell, clCell, linkStyleWithBg)
			} else {
				linkStyleWithBg, _ := f.NewStyle(&excelize.Style{
					Font: &excelize.Font{Color: "0563C1", Underline: "single"},
					Fill: excelize.Fill{Type: "pattern", Color: []string{"FF9999"}, Pattern: 1},
					Border: []excelize.Border{
						{Type: "left", Color: "000000", Style: 1},
						{Type: "right", Color: "000000", Style: 1},
						{Type: "top", Color: "000000", Style: 1},
						{Type: "bottom", Color: "000000", Style: 1},
					},
				})
				f.SetCellStyle(sheetName, clCell, clCell, linkStyleWithBg)
			}
		} else {
			// Apply the same background style even if no link
			f.SetCellValue(sheetName, fmt.Sprintf("I%d", row), "")
			f.SetCellStyle(sheetName, fmt.Sprintf("I%d", row), fmt.Sprintf("I%d", row), style)
		}
	}

	// Enable auto-filter
	if len(results) > 0 {
		f.AutoFilter(sheetName, fmt.Sprintf("A1:I%d", len(results)+1), []excelize.AutoFilterOptions{})
	}

	// Freeze top row
	f.SetPanes(sheetName, &excelize.Panes{
		Freeze:      true,
		XSplit:      0,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
	})

	return nil
}

// createDetailedAnalysisSheet creates the detailed analysis sheet with full reasoning
func createDetailedAnalysisSheet(f *excelize.File, sheetName string, results []models.ApplicantResult) error {
	// Set column widths
	f.SetColWidth(sheetName, "A", "A", 8)
	f.SetColWidth(sheetName, "B", "B", 25)
	f.SetColWidth(sheetName, "C", "C", 20)
	f.SetColWidth(sheetName, "D", "D", 60)

	// Create header style
	headerStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"4472C4"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border: []excelize.Border{
			{Type: "left", Color: "000000", Style: 1},
			{Type: "right", Color: "000000", Style: 1},
			{Type: "top", Color: "000000", Style: 1},
			{Type: "bottom", Color: "000000", Style: 1},
		},
	})
	if err != nil {
		return err
	}

	// Create text wrap style
	wrapStyle, _ := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{WrapText: true, Vertical: "top"},
		Border: []excelize.Border{
			{Type: "left", Color: "000000", Style: 1},
			{Type: "right", Color: "000000", Style: 1},
			{Type: "top", Color: "000000", Style: 1},
			{Type: "bottom", Color: "000000", Style: 1},
		},
	})

	// Set headers
	headers := []string{"Rank", "Candidate", "Category", "Reasoning"}
	for col, header := range headers {
		cell := fmt.Sprintf("%s1", string(rune('A'+col)))
		f.SetCellValue(sheetName, cell, header)
		f.SetCellStyle(sheetName, cell, cell, headerStyle)
	}

	// Populate data
	row := 2
	for _, result := range results {
		// Experience
		f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), result.Rank)
		f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), result.Name)
		f.SetCellValue(sheetName, fmt.Sprintf("C%d", row), "Experience")
		f.SetCellValue(sheetName, fmt.Sprintf("D%d", row), result.Scores.ExperienceReasoning)
		f.SetCellStyle(sheetName, fmt.Sprintf("A%d", row), fmt.Sprintf("D%d", row), wrapStyle)
		f.SetRowHeight(sheetName, row, 60)
		row++

		// Education
		f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), result.Rank)
		f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), result.Name)
		f.SetCellValue(sheetName, fmt.Sprintf("C%d", row), "Education")
		f.SetCellValue(sheetName, fmt.Sprintf("D%d", row), result.Scores.EducationReasoning)
		f.SetCellStyle(sheetName, fmt.Sprintf("A%d", row), fmt.Sprintf("D%d", row), wrapStyle)
		f.SetRowHeight(sheetName, row, 60)
		row++

		// Duties
		f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), result.Rank)
		f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), result.Name)
		f.SetCellValue(sheetName, fmt.Sprintf("C%d", row), "Duties")
		f.SetCellValue(sheetName, fmt.Sprintf("D%d", row), result.Scores.DutiesReasoning)
		f.SetCellStyle(sheetName, fmt.Sprintf("A%d", row), fmt.Sprintf("D%d", row), wrapStyle)
		f.SetRowHeight(sheetName, row, 60)
		row++

		// Cover Letter
		f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), result.Rank)
		f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), result.Name)
		f.SetCellValue(sheetName, fmt.Sprintf("C%d", row), "Cover Letter")
		f.SetCellValue(sheetName, fmt.Sprintf("D%d", row), result.Scores.CoverLetterReasoning)
		f.SetCellStyle(sheetName, fmt.Sprintf("A%d", row), fmt.Sprintf("D%d", row), wrapStyle)
		f.SetRowHeight(sheetName, row, 60)
		row++
	}

	// Freeze top row
	f.SetPanes(sheetName, &excelize.Panes{
		Freeze:      true,
		XSplit:      0,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
	})

	return nil
}
