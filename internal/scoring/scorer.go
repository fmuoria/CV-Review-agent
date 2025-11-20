package scoring

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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

// ScoreApplicant evaluates an applicant against a job description
func (s *Scorer) ScoreApplicant(ctx context.Context, applicant models.ApplicantDocument, jobDesc models.JobDescription) (models.Scores, error) {
	// Build the comprehensive prompt for the LLM
	prompt := s.buildScoringPrompt(applicant, jobDesc)

	// Get response from LLM
	response, err := s.llmClient.GenerateContent(ctx, prompt)
	if err != nil {
		return models.Scores{}, fmt.Errorf("failed to get LLM response: %w", err)
	}

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
	sb.WriteString(applicant.CVContent)
	sb.WriteString("\n\n")

	if applicant.CLContent != "" {
		sb.WriteString("### COVER LETTER CONTENT\n")
		sb.WriteString(applicant.CLContent)
		sb.WriteString("\n\n")
	}

	sb.WriteString("## EVALUATION INSTRUCTIONS\n")
	sb.WriteString("Evaluate the applicant and provide scores with detailed reasoning. Missing REQUIRED qualifications should significantly impact scores, while missing NICE TO HAVE qualifications should have minimal impact.\n\n")
	sb.WriteString("Provide your evaluation in the following JSON format:\n")
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
	sb.WriteString("Return ONLY the JSON object, no additional text.\n")

	return sb.String()
}

// parseScores extracts scores from LLM response
func (s *Scorer) parseScores(response string) (models.Scores, error) {
	// Find JSON in response (in case there's extra text)
	startIdx := strings.Index(response, "{")
	endIdx := strings.LastIndex(response, "}")

	if startIdx == -1 || endIdx == -1 {
		return models.Scores{}, fmt.Errorf("no JSON found in response")
	}

	jsonStr := response[startIdx : endIdx+1]

	var scores models.Scores
	if err := json.Unmarshal([]byte(jsonStr), &scores); err != nil {
		return models.Scores{}, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	return scores, nil
}
