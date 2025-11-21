package scoring

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"unicode/utf8"

	"github.com/fmuoria/CV-Review-agent/internal/llm"
	"github.com/fmuoria/CV-Review-agent/internal/models"
)

// Scorer evaluates applicants using LLM
type Scorer struct {
	llmClient *llm.VertexAIClient
}

// NewScorer creates a new scorer instance
func NewScorer(llmClient *llm.VertexAIClient) *Scorer {
	return &Scorer{
		llmClient: llmClient,
	}
}

// sanitizeUTF8 removes invalid UTF-8 sequences and replaces them with the Unicode replacement character
// This prevents gRPC marshaling errors when sending text to Vertex AI
func sanitizeUTF8(s string) string {
	// If the string is already valid UTF-8, return it as-is
	if utf8.ValidString(s) {
		return s
	}

	// Use strings.ToValidUTF8 to replace invalid UTF-8 sequences with the replacement character (�)
	// This is the most efficient and standard way to clean invalid UTF-8
	return strings.ToValidUTF8(s, "�")
}

// condenseRequirements summarizes a list of requirements into top N items
// This reduces prompt length while preserving key information
func (s *Scorer) condenseRequirements(category string, items []string, maxItems int) string {
	if len(items) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s: ", category))

	// Take top N items or all if less than N
	count := len(items)
	if count > maxItems {
		count = maxItems
	}

	for i := 0; i < count; i++ {
		if i > 0 {
			sb.WriteString("; ")
		}
		sb.WriteString(items[i])
	}

	// Indicate if items were truncated
	if len(items) > maxItems {
		sb.WriteString(fmt.Sprintf(" (+%d more)", len(items)-maxItems))
	}
	sb.WriteString("\n")

	return sb.String()
}

// ScoreApplicant evaluates an applicant against a job description
func (s *Scorer) ScoreApplicant(ctx context.Context, applicant models.ApplicantDocument, jobDesc models.JobDescription) (models.Scores, error) {
	// Build the comprehensive prompt for the LLM
	prompt := s.buildScoringPrompt(applicant, jobDesc)

	// Log request details
	log.Printf("CV length: %d bytes, Cover letter: %d bytes", len(applicant.CVContent), len(applicant.CLContent))
	log.Printf("Sending request to Gemini 2.5 Flash...")

	// Get response from LLM
	response, err := s.llmClient.GenerateContent(ctx, prompt)
	if err != nil {
		return models.Scores{}, fmt.Errorf("failed to get LLM response: %w", err)
	}

	log.Printf("Response received (length: %d bytes)", len(response))
	log.Printf("DEBUG - Raw LLM Response:\n%s", response)

	// Parse the structured response
	scores, err := s.parseScores(response)
	if err != nil {
		return models.Scores{}, fmt.Errorf("failed to parse scores: %w", err)
	}

	// Calculate total score
	scores.TotalScore = scores.ExperienceScore + scores.EducationScore + scores.DutiesScore + scores.CoverLetterScore

	return scores, nil
}

// buildScoringPrompt creates a detailed prompt for the LLM
func (s *Scorer) buildScoringPrompt(applicant models.ApplicantDocument, jobDesc models.JobDescription) string {
	var sb strings.Builder

	sb.WriteString("You are an expert HR analyst evaluating a job applicant. Analyze the following information and provide detailed scoring.\n\n")

	sb.WriteString("## JOB DESCRIPTION\n")
	sb.WriteString(fmt.Sprintf("Title: %s\n", jobDesc.Title))
	sb.WriteString(fmt.Sprintf("Description: %s\n\n", truncate(jobDesc.Description, 500)))

	// Condense requirements to 3-5 key points each instead of listing all items
	sb.WriteString("### REQUIRED QUALIFICATIONS (Must Have - Higher Weight)\n")
	sb.WriteString(s.condenseRequirements("Experience", jobDesc.RequiredExperience, 3))
	sb.WriteString(s.condenseRequirements("Education", jobDesc.RequiredEducation, 3))
	sb.WriteString(s.condenseRequirements("Duties", jobDesc.RequiredDuties, 3))

	sb.WriteString("\n### NICE TO HAVE QUALIFICATIONS (Optional - Lower Weight)\n")
	sb.WriteString(s.condenseRequirements("Experience", jobDesc.NiceToHaveExperience, 2))
	sb.WriteString(s.condenseRequirements("Education", jobDesc.NiceToHaveEducation, 2))
	sb.WriteString(s.condenseRequirements("Duties", jobDesc.NiceToHaveDuties, 2))

	sb.WriteString("\n## APPLICANT INFORMATION\n")
	sb.WriteString(fmt.Sprintf("Name: %s\n\n", applicant.Name))

	sb.WriteString("### CV CONTENT\n")
	// Sanitize and truncate CV content to prevent UTF-8 encoding errors and excessive length
	cvContent := applicant.CVContent
	if !utf8.ValidString(cvContent) {
		log.Printf("Sanitizing invalid UTF-8 in CV for applicant: %s (length: %d bytes)", applicant.Name, len(cvContent))
		cvContent = sanitizeUTF8(cvContent)
		log.Printf("After sanitization: %d bytes", len(cvContent))
	}
	// Truncate CV to 15000 chars max
	if len(cvContent) > 15000 {
		log.Printf("Truncating CV for applicant: %s from %d to 15000 chars", applicant.Name, len(cvContent))
		cvContent = cvContent[:15000] + "\n...[CV truncated for length]"
	}
	sb.WriteString(cvContent)
	sb.WriteString("\n\n")

	if applicant.CLContent != "" {
		sb.WriteString("### COVER LETTER CONTENT\n")
		// Sanitize and truncate cover letter content
		clContent := applicant.CLContent
		if !utf8.ValidString(clContent) {
			log.Printf("Sanitizing invalid UTF-8 in cover letter for applicant: %s (length: %d bytes)", applicant.Name, len(clContent))
			clContent = sanitizeUTF8(clContent)
			log.Printf("After sanitization: %d bytes", len(clContent))
		}
		// Truncate cover letter to 5000 chars max
		if len(clContent) > 5000 {
			log.Printf("Truncating cover letter for applicant: %s from %d to 5000 chars", applicant.Name, len(clContent))
			clContent = clContent[:5000] + "\n...[Cover letter truncated for length]"
		}
		sb.WriteString(clContent)
		sb.WriteString("\n\n")
	}

	sb.WriteString("## EVALUATION\n")
	sb.WriteString("Score the applicant. Missing REQUIRED items = major deductions. Missing NICE TO HAVE = minor impact.\n\n")

	sb.WriteString("OUTPUT: Return ONLY valid JSON (no markdown, no text):\n")
	sb.WriteString("{\n")
	sb.WriteString(`  "experience_score": <0-50>,` + "\n")
	sb.WriteString(`  "experience_reasoning": "<concise 1-2 sentence explanation>",` + "\n")
	sb.WriteString(`  "education_score": <0-20>,` + "\n")
	sb.WriteString(`  "education_reasoning": "<concise 1-2 sentence explanation>",` + "\n")
	sb.WriteString(`  "duties_score": <0-20>,` + "\n")
	sb.WriteString(`  "duties_reasoning": "<concise 1-2 sentence explanation>",` + "\n")
	sb.WriteString(`  "cover_letter_score": <0-10>,` + "\n")
	sb.WriteString(`  "cover_letter_reasoning": "<concise 1-2 sentence explanation>"` + "\n")
	sb.WriteString("}\n")

	return sb.String()
}

// parseScores extracts scores from LLM response
func (s *Scorer) parseScores(response string) (models.Scores, error) {
	log.Printf("DEBUG - Attempting to parse response (length: %d)", len(response))
	log.Printf("DEBUG - Response preview: %s", truncate(response, 500))

	// Strip markdown code blocks if present
	cleanedResponse := response
	if strings.Contains(response, "```json") {
		// Remove ```json prefix and ``` suffix
		cleanedResponse = strings.TrimSpace(response)
		// Try to remove ```json first, if not present try just ```
		if strings.HasPrefix(cleanedResponse, "```json") {
			cleanedResponse = strings.TrimPrefix(cleanedResponse, "```json")
		} else {
			cleanedResponse = strings.TrimPrefix(cleanedResponse, "```")
		}
		cleanedResponse = strings.TrimSuffix(cleanedResponse, "```")
		cleanedResponse = strings.TrimSpace(cleanedResponse)
		log.Printf("DEBUG - Stripped markdown code blocks")
	}

	// Try direct parsing first (response is pure JSON)
	var scores models.Scores
	if err := json.Unmarshal([]byte(cleanedResponse), &scores); err == nil {
		log.Printf("DEBUG - Direct JSON parse successful: Exp=%.2f, Edu=%.2f, Duties=%.2f, CL=%.2f",
			scores.ExperienceScore, scores.EducationScore, scores.DutiesScore, scores.CoverLetterScore)
		return scores, nil
	} else {
		log.Printf("DEBUG - Direct JSON parse failed: %v", err)
	}

	// Try finding JSON between curly braces (response has extra text)
	startIdx := strings.Index(cleanedResponse, "{")
	endIdx := strings.LastIndex(cleanedResponse, "}")

	if startIdx == -1 || endIdx == -1 || startIdx >= endIdx {
		return models.Scores{}, fmt.Errorf("no JSON found in response: %s", truncate(response, 200))
	}

	jsonStr := cleanedResponse[startIdx : endIdx+1]

	if err := json.Unmarshal([]byte(jsonStr), &scores); err != nil {
		log.Printf("DEBUG - Extracted JSON parse failed: %v", err)
		log.Printf("DEBUG - Extracted JSON: %s", jsonStr)
		return models.Scores{}, fmt.Errorf("failed to parse extracted JSON: %w\nExtracted: %s", err, truncate(jsonStr, 200))
	} else {
		log.Printf("DEBUG - Extracted JSON parse successful: Exp=%.2f, Edu=%.2f, Duties=%.2f, CL=%.2f",
			scores.ExperienceScore, scores.EducationScore, scores.DutiesScore, scores.CoverLetterScore)
	}

	return scores, nil
}

// truncate returns the first maxLen characters of s, appending "..." if truncated
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
