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

	sb.WriteString("## CRITICAL SCORING INSTRUCTIONS\n\n")
	sb.WriteString("CURRENT DATE FOR REFERENCE: November 22, 2025 (2025-11-22)\n\n")

	sb.WriteString("### 1. DATE EXTRACTION RULES\n\n")
	sb.WriteString("**Supported Formats:**\n")
	sb.WriteString("1. MM/YYYY → \"08/2025\" = August 2025\n")
	sb.WriteString("2. Month YYYY → \"August 2025\", \"Aug 2025\"\n")
	sb.WriteString("3. YYYY-MM → \"2025-08\"\n")
	sb.WriteString("4. MM/DD/YYYY → \"08/15/2025\" = August 15, 2025\n")
	sb.WriteString("5. DD/MM/YYYY → \"15/08/2025\" = August 15, 2025\n")
	sb.WriteString("6. Year only → \"2021\" = assume January-December 2021\n")
	sb.WriteString("7. \"Present\", \"Current\", \"Ongoing\" → November 22, 2025\n\n")

	sb.WriteString("**Date Range Separators:**\n")
	sb.WriteString("- Recognize: \"-\", \"to\", \"–\", \"—\", \"until\", \"till\"\n")
	sb.WriteString("- Example: \"08/2025-Present\", \"2021 to 2025\", \"Jan 2020–Dec 2023\"\n\n")

	sb.WriteString("**Parsing Algorithm:**\n")
	sb.WriteString("Step 1: Find the 4-digit YEAR (2020, 2021, 2025)\n")
	sb.WriteString("Step 2: Identify MONTH (1-12 or name)\n")
	sb.WriteString("Step 3: If ambiguous (like 08/15/2025 vs 15/08/2025):\n")
	sb.WriteString("   - If first number >12, it's DD/MM/YYYY\n")
	sb.WriteString("   - If both ≤12, assume MM/DD/YYYY\n")
	sb.WriteString("Step 4: Convert \"Present\" → November 2025\n\n")

	sb.WriteString("**Duration Calculation:**\n")
	sb.WriteString("Formula: Duration (months) = (End Year - Start Year) × 12 + (End Month - Start Month)\n\n")
	sb.WriteString("Examples:\n")
	sb.WriteString("- \"02/2021 to 06/2025\" → (2025-2021)×12 + (6-2) = 48+4 = 52 months = 4.3 years\n")
	sb.WriteString("- \"08/2025 to Present\" → (2025-2025)×12 + (11-8) = 0+3 = 3 months = 0.25 years\n")
	sb.WriteString("- \"2018 to Present\" → (2025-2018)×12 + (11-1) = 84+10 = 94 months = 7.8 years\n\n")

	sb.WriteString("**Validation:**\n")
	sb.WriteString("- If end date < start date → FLAG ERROR\n")
	sb.WriteString("- If start date > November 2025 → INVALID (future date)\n")
	sb.WriteString("- If duration > 600 months (50 years) → Likely parsing error\n\n")

	sb.WriteString("### 2. CV DOCUMENT SCANNING RULES\n\n")
	sb.WriteString("**Full Document Review:**\n")
	sb.WriteString("- Scan the ENTIRE document from top to bottom\n")
	sb.WriteString("- Read ALL text including headers, footers, sidebars\n")
	sb.WriteString("- Don't stop after finding first section\n")
	sb.WriteString("- Information might be in columns, tables, or text boxes\n\n")

	sb.WriteString("**Section Detection:**\n")
	sb.WriteString("Look for these headers (case-insensitive, might be bold/caps):\n")
	sb.WriteString("- Experience: \"Work Experience\", \"Experience\", \"Work History\", \"Employment\", \"Professional Experience\"\n")
	sb.WriteString("- Education: \"Education\", \"Academic Background\", \"Qualifications\", \"Academic Qualifications\"\n")
	sb.WriteString("- Skills: \"Skills\", \"Competencies\", \"Technical Skills\", \"Key Skills\"\n")
	sb.WriteString("- Certifications: \"Certifications\", \"Certificates\", \"Professional Development\"\n\n")

	sb.WriteString("**No Assumptions About Order:**\n")
	sb.WriteString("- Education might come before or after experience\n")
	sb.WriteString("- Skills might be at top or bottom\n")
	sb.WriteString("- Contact info might be in header, footer, or main body\n\n")

	sb.WriteString("**Formatting Variations:**\n")
	sb.WriteString("- Bullet points: •, -, *, >, →, ○, ■, ▪\n")
	sb.WriteString("- Dates might be: right-aligned, in columns, in parentheses, after job title\n")
	sb.WriteString("- Multi-column layouts: Read left-to-right, top-to-bottom\n\n")

	sb.WriteString("**Information Extraction:**\n")
	sb.WriteString("- Name: Usually at top, might be larger font\n")
	sb.WriteString("- Contact: Email (look for @), phone (look for digits/+), location (city/country)\n")
	sb.WriteString("- Job titles: Usually bold or prominent near company name\n")
	sb.WriteString("- Dates: Look for patterns like MM/YYYY, might be right-aligned\n")
	sb.WriteString("- Duties: Usually bullet points under job title\n\n")

	sb.WriteString("### 3. JOB TITLE RELEVANCE CHECKING\n\n")
	sb.WriteString("**Step 1: Extract Required Job Roles from Job Description**\n\n")
	if len(jobDesc.RequiredExperience) > 0 {
		sb.WriteString(fmt.Sprintf("For this job: \"%s\"\n", jobDesc.Title))
		sb.WriteString("Required experience keywords: ")
		for i := 0; i < min(3, len(jobDesc.RequiredExperience)); i++ {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(jobDesc.RequiredExperience[i])
		}
		sb.WriteString("\n\n")
	}

	sb.WriteString("**Step 2: Check CV for Matching Job Titles**\n\n")
	sb.WriteString("STRONG MATCH (score 40-50/50 if meets years):\n")
	sb.WriteString("- Exact match of required job titles or roles\n")
	sb.WriteString("- Direct variation: \"Senior Loan Officer\", \"Group Lending Officer\"\n\n")

	sb.WriteString("MODERATE MATCH (score 25-35/50):\n")
	sb.WriteString("- Adjacent role: \"Bank Officer\" (if duties show lending), \"Finance Officer\" (if duties show credit)\n")
	sb.WriteString("- Must have lending duties listed, not just title\n\n")

	sb.WriteString("WEAK MATCH (score 10-20/50):\n")
	sb.WriteString("- Tangential: \"Bank Teller\", \"Finance Intern\", \"Accounts Officer\"\n")
	sb.WriteString("- Financial role but no lending responsibilities\n\n")

	sb.WriteString("NO MATCH (score 0-10/50):\n")
	sb.WriteString("- Unrelated title: \"Supply Chain Manager\", \"Sales Representative\", \"Agriculture Officer\"\n")
	sb.WriteString("- Even if they mention \"credit\" in passing (e.g., \"negotiated credit terms\" in sales)\n\n")

	sb.WriteString("**Step 3: Validate With Duties**\n\n")
	sb.WriteString("Job title alone isn't enough. Check duties:\n")
	sb.WriteString("- Loan Officer with duties: \"loan disbursement, group formation, VSLA\" → VALID\n")
	sb.WriteString("- Finance Officer with duties: \"bookkeeping, invoicing, payments\" → NOT lending\n")
	sb.WriteString("- Sales Rep with: \"negotiated credit terms\" → NOT lending (just payment terms)\n\n")

	sb.WriteString("**Critical: Keyword in wrong context ≠ Experience**\n")
	sb.WriteString("❌ \"Negotiated credit terms with customers\" (sales) ≠ Loan officer\n")
	sb.WriteString("❌ \"Processed credit memos\" (accounting) ≠ Credit analyst\n")
	sb.WriteString("❌ \"Managed supplier credit\" (procurement) ≠ Lending officer\n\n")

	sb.WriteString("### 4. EXPERIENCE SCORING (0-50 points)\n\n")
	sb.WriteString("**FIRST: Check Relevance (Job Title + Duties)**\n")
	sb.WriteString("If NO relevant job title found → MAX 10 points regardless of years\n\n")

	sb.WriteString("**THEN: Score Based on Duration (Only if relevant)**\n\n")
	sb.WriteString("Duration Tiers (for RELEVANT experience only):\n")
	sb.WriteString("- 0-6 months: Entry-level → 18-24/50\n")
	sb.WriteString("- 6-12 months: Junior → 24-28/50\n")
	sb.WriteString("- 12-24 months: Intermediate → 28-34/50\n")
	sb.WriteString("- 24-36 months: Mid-level → 34-40/50\n")
	sb.WriteString("- 36-60 months: Senior → 40-45/50\n")
	sb.WriteString("- 60+ months: Expert → 45-50/50\n\n")

	sb.WriteString("**Scoring Examples for This Job:**\n\n")
	if len(jobDesc.RequiredExperience) > 0 {
		sb.WriteString(fmt.Sprintf("Job requirement: \"%s\"\n\n", jobDesc.RequiredExperience[0]))
	}

	sb.WriteString("Example A:\n")
	sb.WriteString("\"Loan Officer – ASA Kenya (08/2025-Present)\"\n")
	sb.WriteString("Calculation: Aug 2025 to Nov 2025 = 3 months\n")
	sb.WriteString("Category: Entry-level (0-6 months)\n")
	sb.WriteString("Score: 22/50\n")
	sb.WriteString("Reasoning: \"Highly relevant job title and duties, but only 3 months in role. Far short of multi-year requirement.\"\n\n")

	sb.WriteString("Example B:\n")
	sb.WriteString("\"Supply Chain Manager (2015-Present)\"\n")
	sb.WriteString("Calculation: 2015 to Nov 2025 = 120 months = 10 years\n")
	sb.WriteString("Category: NOT RELEVANT (no lending job title)\n")
	sb.WriteString("Score: 8/50\n")
	sb.WriteString("Reasoning: \"10 years experience but in supply chain, not lending. No relevant job title or duties.\"\n\n")

	sb.WriteString("Example C:\n")
	sb.WriteString("\"Microfinance Officer (2018-Present)\"\n")
	sb.WriteString("Calculation: 2018 to Nov 2025 = 84 months = 7 years\n")
	sb.WriteString("Category: Expert (60+ months)\n")
	sb.WriteString("Score: 47/50\n")
	sb.WriteString("Reasoning: \"7 years in microfinance, exceeding multi-year requirement.\"\n\n")

	sb.WriteString("Example D:\n")
	sb.WriteString("\"Finance Officer (02/2021-06/2025)\" with duties: \"bank reconciliation, invoicing\"\n")
	sb.WriteString("Calculation: 52 months = 4.3 years\n")
	sb.WriteString("Category: NOT RELEVANT (no lending duties)\n")
	sb.WriteString("Score: 12/50\n")
	sb.WriteString("Reasoning: \"4.3 years but in general accounting, not lending. No credit/loan duties.\"\n\n")

	sb.WriteString("### 5. ACCURACY CHECKS\n\n")
	sb.WriteString("**Date Validation:**\n")
	sb.WriteString("✓ End date must be ≥ start date\n")
	sb.WriteString("✓ Start date must be ≤ November 2025\n")
	sb.WriteString("✓ If duration > 50 years, flag as parsing error\n")
	sb.WriteString("✓ If dates are out of chronological order, note the inconsistency\n\n")

	sb.WriteString("**Information Completeness:**\n")
	sb.WriteString("If information is missing, explicitly state:\n")
	sb.WriteString("- \"Education: Not found in CV\"\n")
	sb.WriteString("- \"Email: Not provided\"\n")
	sb.WriteString("- \"Job dates: Not specified\"\n\n")
	sb.WriteString("DO NOT guess or infer information that isn't present.\n\n")

	sb.WriteString("**Overlap Detection:**\n")
	sb.WriteString("If candidate lists 3+ full-time roles with overlapping dates:\n")
	sb.WriteString("- Flag as potentially suspicious\n")
	sb.WriteString("- Note: \"CV shows 3 concurrent full-time roles which is unusual\"\n\n")

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
