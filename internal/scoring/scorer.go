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
	sb.WriteString(fmt.Sprintf("Description: %s\n\n", jobDesc.Description))

	sb.WriteString("### REQUIRED QUALIFICATIONS (Must Have - Higher Weight)\n")
	if len(jobDesc.RequiredExperience) > 0 {
		sb.WriteString("Required Experience:\n")
		for _, exp := range jobDesc.RequiredExperience {
			sb.WriteString(fmt.Sprintf("- %s\n", exp))
		}
	}
	if len(jobDesc.RequiredEducation) > 0 {
		sb.WriteString("Required Education:\n")
		for _, edu := range jobDesc.RequiredEducation {
			sb.WriteString(fmt.Sprintf("- %s\n", edu))
		}
	}
	if len(jobDesc.RequiredDuties) > 0 {
		sb.WriteString("Required Duties:\n")
		for _, duty := range jobDesc.RequiredDuties {
			sb.WriteString(fmt.Sprintf("- %s\n", duty))
		}
	}

	sb.WriteString("\n### NICE TO HAVE QUALIFICATIONS (Optional - Lower Weight)\n")
	if len(jobDesc.NiceToHaveExperience) > 0 {
		sb.WriteString("Nice to Have Experience:\n")
		for _, exp := range jobDesc.NiceToHaveExperience {
			sb.WriteString(fmt.Sprintf("- %s\n", exp))
		}
	}
	if len(jobDesc.NiceToHaveEducation) > 0 {
		sb.WriteString("Nice to Have Education:\n")
		for _, edu := range jobDesc.NiceToHaveEducation {
			sb.WriteString(fmt.Sprintf("- %s\n", edu))
		}
	}
	if len(jobDesc.NiceToHaveDuties) > 0 {
		sb.WriteString("Nice to Have Duties:\n")
		for _, duty := range jobDesc.NiceToHaveDuties {
			sb.WriteString(fmt.Sprintf("- %s\n", duty))
		}
	}

	sb.WriteString("\n## APPLICANT INFORMATION\n")
	sb.WriteString(fmt.Sprintf("Name: %s\n\n", applicant.Name))

	sb.WriteString("### CV CONTENT\n")
	// Sanitize CV content to prevent UTF-8 encoding errors
	cvContent := applicant.CVContent
	if !utf8.ValidString(cvContent) {
		log.Printf("Sanitizing invalid UTF-8 in CV for applicant: %s (length: %d bytes)", applicant.Name, len(cvContent))
		cvContent = sanitizeUTF8(cvContent)
		log.Printf("After sanitization: %d bytes", len(cvContent))
	}
	sb.WriteString(cvContent)
	sb.WriteString("\n\n")

	if applicant.CLContent != "" {
		sb.WriteString("### COVER LETTER CONTENT\n")
		// Sanitize cover letter content to prevent UTF-8 encoding errors
		clContent := applicant.CLContent
		if !utf8.ValidString(clContent) {
			log.Printf("Sanitizing invalid UTF-8 in cover letter for applicant: %s (length: %d bytes)", applicant.Name, len(clContent))
			clContent = sanitizeUTF8(clContent)
			log.Printf("After sanitization: %d bytes", len(clContent))
		}
		sb.WriteString(clContent)
		sb.WriteString("\n\n")
	}

	sb.WriteString("## EVALUATION INSTRUCTIONS\n")
	sb.WriteString("Evaluate the applicant and provide scores with detailed reasoning. Missing REQUIRED qualifications should significantly impact scores, while missing NICE TO HAVE qualifications should have minimal impact.\n\n")

	sb.WriteString("CRITICAL OUTPUT REQUIREMENTS:\n")
	sb.WriteString("1. Your response MUST be ONLY a valid JSON object\n")
	sb.WriteString("2. Do NOT include any explanatory text before or after the JSON\n")
	sb.WriteString("3. Do NOT use markdown code blocks (no ```json or ```)\n")
	sb.WriteString("4. Return ONLY the raw JSON object starting with { and ending with }\n\n")

	sb.WriteString("REQUIRED JSON FORMAT:\n")
	sb.WriteString("{\n")
	sb.WriteString(`  "experience_score": <0-50>,` + "\n")
	sb.WriteString(`  "experience_reasoning": "<detailed explanation of experience match, highlighting required vs nice-to-have>",` + "\n")
	sb.WriteString(`  "education_score": <0-20>,` + "\n")
	sb.WriteString(`  "education_reasoning": "<detailed explanation of education match, highlighting required vs nice-to-have>",` + "\n")
	sb.WriteString(`  "duties_score": <0-20>,` + "\n")
	sb.WriteString(`  "duties_reasoning": "<detailed explanation of duties/responsibilities match>",` + "\n")
	sb.WriteString(`  "cover_letter_score": <0-10>,` + "\n")
	sb.WriteString(`  "cover_letter_reasoning": "<detailed explanation of cover letter quality and alignment>"` + "\n")
	sb.WriteString("}\n\n")

	sb.WriteString("SCORING CRITERIA:\n")
	sb.WriteString("- Experience Score (0-50): Weight heavily towards required experience. Each missing required qualification should reduce score by 10-15 points. Missing nice-to-have should reduce by 2-5 points.\n")
	sb.WriteString("- Education Score (0-20): Required education is critical. Missing required degree should reduce score by 10+ points. Missing nice-to-have should reduce by 2-3 points.\n")
	sb.WriteString("- Duties Score (0-20): Evaluate ability to perform required duties. Each unmet required duty should reduce score by 5-7 points.\n")
	sb.WriteString("- Cover Letter Score (0-10): Assess quality, relevance, and alignment with role. No cover letter = 0 points.\n\n")
	sb.WriteString("NOW EVALUATE THE CANDIDATE AND RETURN ONLY THE JSON OBJECT WITH SCORES (0-100) AND REASONING.\n")

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
		cleanedResponse = strings.TrimPrefix(cleanedResponse, "```json")
		cleanedResponse = strings.TrimPrefix(cleanedResponse, "```")
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
	}

	log.Printf("DEBUG - Extracted JSON parse successful: Exp=%.2f, Edu=%.2f, Duties=%.2f, CL=%.2f",
		scores.ExperienceScore, scores.EducationScore, scores.DutiesScore, scores.CoverLetterScore)

	return scores, nil
}

// truncate returns the first maxLen characters of s, appending "..." if truncated
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
