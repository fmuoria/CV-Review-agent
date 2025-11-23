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
	sb.WriteString("7. \"Present\", \"Current\", \"Ongoing\" → November 22, 2025\n")
	sb.WriteString("8. With apostrophes: \"Jan '21\", \"'21\"\n")
	sb.WriteString("9. Ranges without months: \"2020-2024\" → assume full years\n")
	sb.WriteString("10. Quarter format: \"Q1 2024\" → January 2024\n")
	sb.WriteString("11. Fiscal year: \"FY 2024\" → treat as calendar 2024\n")
	sb.WriteString("12. Approximate: \"circa 2020\", \"around 2021\" → use stated year\n\n")

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

	sb.WriteString("**SKILLS EXTRACTION STRATEGY:**\n\n")
	sb.WriteString("Look for skills in MULTIPLE locations throughout the CV:\n")
	sb.WriteString("1. Dedicated Skills/Competencies/Technical Skills section\n")
	sb.WriteString("2. Embedded in job duties and responsibilities\n")
	sb.WriteString("3. Listed in achievements and results\n")
	sb.WriteString("4. Mentioned in education, certifications, or training\n")
	sb.WriteString("5. Referenced in project descriptions\n\n")

	sb.WriteString("**For THIS Specific Role, Extract Skills Related To:**\n\n")

	// Dynamically extract skill categories from required experience
	if len(jobDesc.RequiredExperience) > 0 {
		sb.WriteString("Required Experience Keywords:\n")
		for i := 0; i < min(5, len(jobDesc.RequiredExperience)); i++ {
			sb.WriteString(fmt.Sprintf("  • %s\n", jobDesc.RequiredExperience[i]))
		}
		sb.WriteString("\n")
	}

	// Dynamically extract from required duties
	if len(jobDesc.RequiredDuties) > 0 {
		sb.WriteString("Required Duties Keywords:\n")
		for i := 0; i < min(5, len(jobDesc.RequiredDuties)); i++ {
			sb.WriteString(fmt.Sprintf("  • %s\n", jobDesc.RequiredDuties[i]))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("**Skill Matching Instructions:**\n")
	sb.WriteString("1. Identify the CORE skills needed for this role from the requirements above\n")
	sb.WriteString("2. Search the CV for evidence of these skills in ANY section\n")
	sb.WriteString("3. Accept synonyms, related tools, or equivalent technologies\n")
	sb.WriteString("4. Weight hands-on experience higher than theoretical knowledge\n")
	sb.WriteString("5. Look for DEPTH (years used, proficiency level) not just mentions\n\n")

	sb.WriteString("### 3. JOB TITLE RELEVANCE CHECKING\n\n")
	sb.WriteString("**Step 1: Extract Key Roles from THIS Job Description**\n\n")
	sb.WriteString(fmt.Sprintf("Target Job Title: \"%s\"\n", jobDesc.Title))

	// Dynamically extract keywords from required experience
	if len(jobDesc.RequiredExperience) > 0 {
		sb.WriteString("Key experience requirements for this role:\n")
		for i := 0; i < min(5, len(jobDesc.RequiredExperience)); i++ {
			sb.WriteString(fmt.Sprintf("  - %s\n", jobDesc.RequiredExperience[i]))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("**Step 2: Semantic Job Title Matching**\n\n")
	sb.WriteString("Match CV job titles to the target role using these criteria:\n\n")

	sb.WriteString("STRONG MATCH (40-50/50 if meets duration):\n")
	sb.WriteString(fmt.Sprintf("- Exact or near-exact match to \"%s\"\n", jobDesc.Title))
	sb.WriteString("- Direct variations or synonyms of the target title\n")
	sb.WriteString("- Different seniority levels of same role (e.g., Junior/Senior/Lead versions)\n")
	sb.WriteString("- Job titles that perform the SAME core functions\n\n")

	sb.WriteString("MODERATE MATCH (25-35/50):\n")
	sb.WriteString("- Adjacent/related roles in the same field\n")
	sb.WriteString("- Roles that perform SOME of the required duties\n")
	sb.WriteString("- Must show relevant duties in job description, not just title\n\n")

	sb.WriteString("WEAK MATCH (10-20/50):\n")
	sb.WriteString("- Tangentially related roles in similar industry\n")
	sb.WriteString("- Same industry but different function\n")
	sb.WriteString("- Transferable skills but different domain\n\n")

	sb.WriteString("NO MATCH (0-10/50):\n")
	sb.WriteString("- Completely unrelated job titles\n")
	sb.WriteString("- Different industry and different function\n")
	sb.WriteString("- No overlap in skills or responsibilities\n\n")

	sb.WriteString("**Step 3: Validate With Duties Against Required Qualifications**\n\n")
	sb.WriteString("CRITICAL: Job title alone is insufficient. Cross-check with required duties:\n\n")

	// Dynamically reference actual required duties
	if len(jobDesc.RequiredDuties) > 0 {
		sb.WriteString("For THIS job, the CV must show experience with:\n")
		for i := 0; i < min(5, len(jobDesc.RequiredDuties)); i++ {
			sb.WriteString(fmt.Sprintf("  ✓ %s\n", jobDesc.RequiredDuties[i]))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Validation Rules:\n")
	sb.WriteString("- If CV shows matching title + matching duties → VALID, score based on duration\n")
	sb.WriteString("- If CV shows matching title but WRONG duties → NOT VALID, max 15/50\n")
	sb.WriteString("- If CV shows different title but MATCHING duties → Consider MODERATE match\n\n")

	sb.WriteString("**Critical: Keyword in Wrong Context ≠ Experience**\n")
	sb.WriteString("Example patterns to watch:\n")
	sb.WriteString("❌ Keyword mentioned in passing (e.g., \"collaborated with X team\") ≠ X experience\n")
	sb.WriteString("❌ Used tool/process incidentally ≠ Expertise in that area\n")
	sb.WriteString("❌ Overlapping terminology from different context ≠ Relevant experience\n\n")

	sb.WriteString("### 4. QUANTIFIED ACHIEVEMENT MATCHING\n\n")
	sb.WriteString("**Scan for Numeric Achievements That Match Job Requirements:**\n\n")

	// Dynamically build achievement matching from job description
	sb.WriteString("Expected Outcomes from Job Description:\n")

	// Extract numbers from required duties
	if len(jobDesc.RequiredDuties) > 0 {
		for _, duty := range jobDesc.RequiredDuties {
			sb.WriteString(fmt.Sprintf("  • %s\n", duty))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("**Achievement Matching Logic:**\n\n")
	sb.WriteString("1. Extract ALL numbers from CV: percentages, counts, currency, time periods\n")
	sb.WriteString("2. Match CV numbers to job requirement numbers:\n")
	sb.WriteString("   - Look for similar magnitude (if job needs 100, CV showing 80-150 is good)\n")
	sb.WriteString("   - Look for same metric type (participants, retention %, revenue, etc.)\n")
	sb.WriteString("   - Accept equivalent achievements (trained 200 = recruited 200)\n\n")

	sb.WriteString("3. Give BONUS points for matching quantified achievements:\n")
	sb.WriteString("   - Exact or close match: +8 to +10 points\n")
	sb.WriteString("   - Exceeds requirement: +10 to +15 points\n")
	sb.WriteString("   - Below requirement but reasonable: +3 to +5 points\n")
	sb.WriteString("   - No matching numbers found: 0 bonus\n\n")

	sb.WriteString("**Examples of Achievement Matching:**\n")
	sb.WriteString("- Job requires: \"Manage team of 10\" | CV shows: \"Led team of 12\" → Strong match\n")
	sb.WriteString("- Job requires: \"95% satisfaction\" | CV shows: \"Achieved 92% NPS\" → Good match\n")
	sb.WriteString("- Job requires: \"Process 500 applications\" | CV shows: \"Processed 600+ monthly\" → Exceeds\n")
	sb.WriteString("- Job requires: \"Increase revenue 20%\" | CV shows: \"Grew sales 35%\" → Strong evidence\n\n")

	sb.WriteString("### 5. EXPERIENCE SCORING (0-50 points)\n\n")
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

	sb.WriteString("**Scoring Examples for THIS Specific Job:**\n\n")
	sb.WriteString(fmt.Sprintf("Job Title: \"%s\"\n", jobDesc.Title))

	if len(jobDesc.RequiredExperience) > 0 {
		sb.WriteString(fmt.Sprintf("Key Requirement: \"%s\"\n\n", jobDesc.RequiredExperience[0]))
	}

	sb.WriteString("Example A - Strong Match:\n")
	sb.WriteString(fmt.Sprintf("CV shows: Job title matching \"%s\" or close variation\n", jobDesc.Title))
	sb.WriteString("Duration: 3+ years in highly relevant role\n")
	if len(jobDesc.RequiredDuties) > 0 {
		sb.WriteString(fmt.Sprintf("Duties: Demonstrates \"%s\" and other required duties\n", jobDesc.RequiredDuties[0]))
	}
	sb.WriteString("Expected Score: 85-95/100\n")
	sb.WriteString("Reasoning: \"Excellent match with required experience, education, and demonstrated duties.\"\n\n")

	sb.WriteString("Example B - Moderate Match:\n")
	sb.WriteString("CV shows: Related but not identical job title\n")
	sb.WriteString("Duration: 1-2 years in adjacent field\n")
	sb.WriteString("Duties: Shows SOME required duties but missing critical ones\n")
	sb.WriteString("Expected Score: 60-75/100\n")
	sb.WriteString("Reasoning: \"Relevant experience but shorter duration and missing some key requirements.\"\n\n")

	sb.WriteString("Example C - Weak Match:\n")
	sb.WriteString("CV shows: Different job title, same industry\n")
	sb.WriteString("Duration: 5+ years but in wrong function\n")
	sb.WriteString("Duties: Minimal overlap with required duties\n")
	sb.WriteString("Expected Score: 30-50/100\n")
	sb.WriteString("Reasoning: \"Extensive experience but in unrelated role. Few transferable skills.\"\n\n")

	sb.WriteString("Example D - No Match:\n")
	sb.WriteString("CV shows: Unrelated industry and function\n")
	sb.WriteString("Duration: Any duration\n")
	sb.WriteString("Duties: No overlap with requirements\n")
	sb.WriteString("Expected Score: 0-25/100\n")
	sb.WriteString("Reasoning: \"No relevant experience for this position.\"\n\n")

	sb.WriteString("### 6. EDUCATION SCORING (0-20 points)\n\n")

	// Check if education is actually required
	hasRequiredEducation := len(jobDesc.RequiredEducation) > 0
	hasNiceToHaveEducation := len(jobDesc.NiceToHaveEducation) > 0

	if hasRequiredEducation {
		sb.WriteString("**Education IS Required for This Role:**\n\n")
		sb.WriteString("Required Education:\n")
		for _, edu := range jobDesc.RequiredEducation {
			sb.WriteString(fmt.Sprintf("  • %s\n", edu))
		}
		sb.WriteString("\n")

		sb.WriteString("Scoring Guidelines:\n")
		sb.WriteString("- Has ALL required education: 18-20/20\n")
		sb.WriteString("- Has MOST required education: 12-17/20\n")
		sb.WriteString("- Has SOME required education: 8-11/20\n")
		sb.WriteString("- Missing required education: 0-7/20\n")
		sb.WriteString("- PENALTY: -10 to -15 points for each missing required degree/certification\n\n")
	} else {
		sb.WriteString("**Education is NOT Explicitly Required (Field/Experience-Based Role):**\n\n")
		sb.WriteString("Since no specific education is required, use flexible scoring:\n")
		sb.WriteString("- Relevant degree/diploma: 15-20/20\n")
		sb.WriteString("- Any higher education: 10-14/20\n")
		sb.WriteString("- High school + strong experience: 8-12/20\n")
		sb.WriteString("- High school only: 5-7/20\n")
		sb.WriteString("- Prioritize EXPERIENCE over formal education for this role\n\n")
	}

	if hasNiceToHaveEducation {
		sb.WriteString("Nice-to-Have Education (BONUS):\n")
		for _, edu := range jobDesc.NiceToHaveEducation {
			sb.WriteString(fmt.Sprintf("  • %s\n", edu))
		}
		sb.WriteString("- BONUS: +2 to +3 points each (max +5 total)\n\n")
	}

	sb.WriteString("### 7. DUTIES/RESPONSIBILITIES SCORING (0-20 points)\n\n")
	sb.WriteString("**Evaluate Candidate's Ability to Perform Required Duties:**\n\n")

	// List actual required duties
	if len(jobDesc.RequiredDuties) > 0 {
		sb.WriteString("REQUIRED Duties for This Role:\n")
		for i, duty := range jobDesc.RequiredDuties {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, duty))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("**Scoring Method:**\n\n")
	sb.WriteString("For EACH required duty:\n")
	sb.WriteString("1. Search CV for evidence candidate has performed this duty\n")
	sb.WriteString("2. Look for:\n")
	sb.WriteString("   - Exact match: same duty described in CV\n")
	sb.WriteString("   - Semantic match: similar duty with different wording\n")
	sb.WriteString("   - Partial match: related but not identical duty\n\n")

	sb.WriteString("3. Score based on evidence:\n")
	sb.WriteString("   - Strong evidence (multiple examples): Full points for that duty\n")
	sb.WriteString("   - Moderate evidence (one example): 60-80% points\n")
	sb.WriteString("   - Weak evidence (indirect/implied): 30-50% points\n")
	sb.WriteString("   - No evidence: 0 points + PENALTY -5 to -7 points\n\n")

	sb.WriteString("**Calculate Total Duties Score:**\n")
	totalDuties := len(jobDesc.RequiredDuties)
	if totalDuties > 0 {
		pointsPerDuty := 20.0 / float64(totalDuties)
		sb.WriteString(fmt.Sprintf("- %d required duties = %.1f points each\n", totalDuties, pointsPerDuty))
		sb.WriteString("- Sum the points for all duties\n")
		sb.WriteString("- Subtract penalties for missing critical duties\n")
		sb.WriteString("- Maximum score: 20 points\n\n")
	}

	// Optional: Nice-to-have duties
	if len(jobDesc.NiceToHaveDuties) > 0 {
		sb.WriteString("Nice-to-Have Duties (BONUS up to +3 points):\n")
		for _, duty := range jobDesc.NiceToHaveDuties {
			sb.WriteString(fmt.Sprintf("  • %s\n", duty))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("### 8. ACCURACY CHECKS\n\n")
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

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
