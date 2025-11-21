package scoring

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/fmuoria/CV-Review-agent/internal/models"
)

// TestSanitizeUTF8_ValidString tests that valid UTF-8 strings are returned unchanged
func TestSanitizeUTF8_ValidString(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "Simple ASCII text",
			input: "Hello, World!",
		},
		{
			name:  "UTF-8 with special characters",
			input: "José González - Software Engineer with 5+ years of experience in Go, Python, and Java.",
		},
		{
			name:  "UTF-8 with emoji",
			input: "Experienced developer 🚀 with strong communication skills 💻",
		},
		{
			name:  "Multi-language text",
			input: "Software Engineer - 软件工程师 - مهندس برمجيات",
		},
		{
			name:  "Empty string",
			input: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeUTF8(tt.input)
			if result != tt.input {
				t.Errorf("sanitizeUTF8() changed valid UTF-8 string: got %q, want %q", result, tt.input)
			}
			if !utf8.ValidString(result) {
				t.Errorf("sanitizeUTF8() returned invalid UTF-8 string")
			}
		})
	}
}

// TestSanitizeUTF8_InvalidString tests that invalid UTF-8 sequences are fixed
func TestSanitizeUTF8_InvalidString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string // String should contain this after sanitization
	}{
		{
			name:     "Invalid UTF-8 byte sequence at start",
			input:    string([]byte{0xFF, 0xFE}) + "Valid text",
			contains: "Valid text",
		},
		{
			name:     "Invalid UTF-8 byte sequence in middle",
			input:    "Start " + string([]byte{0xFF, 0xFE}) + " End",
			contains: "Start",
		},
		{
			name:     "Multiple invalid sequences",
			input:    string([]byte{0xFF}) + "Text" + string([]byte{0xFE}) + "More",
			contains: "Text",
		},
		{
			name:     "Invalid continuation bytes",
			input:    "Name: John" + string([]byte{0x80, 0x81}),
			contains: "Name: John",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify input is actually invalid
			if utf8.ValidString(tt.input) {
				t.Skip("Test input is valid UTF-8, skipping")
			}

			result := sanitizeUTF8(tt.input)

			// Result must be valid UTF-8
			if !utf8.ValidString(result) {
				t.Errorf("sanitizeUTF8() returned invalid UTF-8 string: %q", result)
			}

			// Result should be non-empty
			if len(result) == 0 {
				t.Errorf("sanitizeUTF8() returned empty string")
			}
		})
	}
}

// TestSanitizeUTF8_PreservesContent tests that sanitization preserves meaningful content
func TestSanitizeUTF8_PreservesContent(t *testing.T) {
	// Create a string with invalid UTF-8 in the middle
	validPart1 := "Education: Bachelor of Science in Computer Science"
	validPart2 := "Experience: 5 years in software development"
	invalidBytes := []byte{0xFF, 0xFE}
	input := validPart1 + string(invalidBytes) + validPart2

	result := sanitizeUTF8(input)

	// Result must be valid UTF-8
	if !utf8.ValidString(result) {
		t.Errorf("sanitizeUTF8() returned invalid UTF-8 string")
	}

	// Result should be non-empty
	if len(result) == 0 {
		t.Errorf("sanitizeUTF8() returned empty string")
	}

	// Result length should be reasonable (not much shorter than input)
	// Allow for replacement characters
	if len(result) < len(input)-10 {
		t.Errorf("sanitizeUTF8() removed too much content: input %d bytes, output %d bytes", len(input), len(result))
	}
}

// TestSanitizeUTF8_ReplacementCharacter tests that invalid sequences are replaced with �
func TestSanitizeUTF8_ReplacementCharacter(t *testing.T) {
	// Create a string with a single invalid byte
	invalidByte := []byte{0xFF}
	input := "Before" + string(invalidByte) + "After"

	result := sanitizeUTF8(input)

	// Result must be valid UTF-8
	if !utf8.ValidString(result) {
		t.Errorf("sanitizeUTF8() returned invalid UTF-8 string")
	}

	// Result should contain the text before and after
	if !strings.Contains(result, "Before") || !strings.Contains(result, "After") {
		t.Errorf("sanitizeUTF8() did not preserve valid text: got %q", result)
	}

	// Result should contain the replacement character
	if !strings.Contains(result, "�") {
		t.Errorf("sanitizeUTF8() did not include replacement character")
	}
}

// TestTruncate tests the truncate helper function
func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "Short string not truncated",
			input:  "Hello",
			maxLen: 10,
			want:   "Hello",
		},
		{
			name:   "Exact length not truncated",
			input:  "Hello",
			maxLen: 5,
			want:   "Hello",
		},
		{
			name:   "Long string truncated",
			input:  "This is a very long string that should be truncated",
			maxLen: 20,
			want:   "This is a very long ...",
		},
		{
			name:   "Empty string",
			input:  "",
			maxLen: 10,
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncate(tt.input, tt.maxLen)
			if result != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, result, tt.want)
			}
		})
	}
}

// TestParseScores_DirectJSON tests parsing of pure JSON responses
func TestParseScores_DirectJSON(t *testing.T) {
	scorer := &Scorer{}

	validJSON := `{
		"experience_score": 45.5,
		"experience_reasoning": "Strong experience",
		"education_score": 18.0,
		"education_reasoning": "Excellent education",
		"duties_score": 19.0,
		"duties_reasoning": "Well matched",
		"cover_letter_score": 8.5,
		"cover_letter_reasoning": "Good cover letter"
	}`

	scores, err := scorer.parseScores(validJSON)
	if err != nil {
		t.Fatalf("parseScores() failed: %v", err)
	}

	if scores.ExperienceScore != 45.5 {
		t.Errorf("ExperienceScore = %v, want 45.5", scores.ExperienceScore)
	}
	if scores.EducationScore != 18.0 {
		t.Errorf("EducationScore = %v, want 18.0", scores.EducationScore)
	}
	if scores.DutiesScore != 19.0 {
		t.Errorf("DutiesScore = %v, want 19.0", scores.DutiesScore)
	}
	if scores.CoverLetterScore != 8.5 {
		t.Errorf("CoverLetterScore = %v, want 8.5", scores.CoverLetterScore)
	}
}

// TestCondenseRequirements tests requirement condensing logic
func TestCondenseRequirements(t *testing.T) {
	scorer := &Scorer{}

	tests := []struct {
		name     string
		category string
		items    []string
		maxItems int
		want     string
	}{
		{
			name:     "Empty list",
			category: "Experience",
			items:    []string{},
			maxItems: 3,
			want:     "",
		},
		{
			name:     "Less than max items",
			category: "Education",
			items:    []string{"Bachelor's degree", "Master's degree"},
			maxItems: 3,
			want:     "Education: Bachelor's degree; Master's degree\n",
		},
		{
			name:     "Exactly max items",
			category: "Duties",
			items:    []string{"Task 1", "Task 2", "Task 3"},
			maxItems: 3,
			want:     "Duties: Task 1; Task 2; Task 3\n",
		},
		{
			name:     "More than max items",
			category: "Experience",
			items:    []string{"Exp 1", "Exp 2", "Exp 3", "Exp 4", "Exp 5"},
			maxItems: 3,
			want:     "Experience: Exp 1; Exp 2; Exp 3 (+2 more)\n",
		},
		{
			name:     "Single item",
			category: "Education",
			items:    []string{"PhD in Computer Science"},
			maxItems: 3,
			want:     "Education: PhD in Computer Science\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := scorer.condenseRequirements(tt.category, tt.items, tt.maxItems)
			if result != tt.want {
				t.Errorf("condenseRequirements() = %q, want %q", result, tt.want)
			}
		})
	}
}

// TestBuildScoringPrompt_ContentTruncation tests that CV and cover letter are truncated
func TestBuildScoringPrompt_ContentTruncation(t *testing.T) {
	scorer := &Scorer{}

	// Create long CV content (> 8000 chars)
	longCV := strings.Repeat("This is CV content. ", 500)           // ~10,000 chars
	longCL := strings.Repeat("This is cover letter content. ", 150) // ~4,500 chars

	applicant := models.ApplicantDocument{
		Name:      "John Doe",
		CVContent: longCV,
		CLContent: longCL,
	}

	jobDesc := models.JobDescription{
		Title:              "Software Engineer",
		Description:        "A great job",
		RequiredExperience: []string{"Go", "Python", "Java"},
	}

	prompt := scorer.buildScoringPrompt(applicant, jobDesc)

	// Check that CV was truncated
	if !strings.Contains(prompt, "[CV truncated for length]") {
		t.Error("Expected CV to be truncated but truncation message not found")
	}

	// Check that cover letter was truncated
	if !strings.Contains(prompt, "[Cover letter truncated for length]") {
		t.Error("Expected cover letter to be truncated but truncation message not found")
	}

	// Ensure prompt is reasonably sized (should be much less than original content)
	if len(prompt) > 15000 {
		t.Errorf("Prompt still too long: %d bytes", len(prompt))
	}
}

// TestBuildScoringPrompt_NoTruncationNeeded tests that short content is not truncated
func TestBuildScoringPrompt_NoTruncationNeeded(t *testing.T) {
	scorer := &Scorer{}

	applicant := models.ApplicantDocument{
		Name:      "Jane Smith",
		CVContent: "Short CV content",
		CLContent: "Short cover letter",
	}

	jobDesc := models.JobDescription{
		Title:              "Data Analyst",
		Description:        "Analyze data",
		RequiredExperience: []string{"SQL", "Excel"},
	}

	prompt := scorer.buildScoringPrompt(applicant, jobDesc)

	// Check that content was NOT truncated
	if strings.Contains(prompt, "[CV truncated for length]") {
		t.Error("CV should not be truncated for short content")
	}

	if strings.Contains(prompt, "[Cover letter truncated for length]") {
		t.Error("Cover letter should not be truncated for short content")
	}

	// Ensure original content is present
	if !strings.Contains(prompt, "Short CV content") {
		t.Error("Original CV content not found in prompt")
	}

	if !strings.Contains(prompt, "Short cover letter") {
		t.Error("Original cover letter content not found in prompt")
	}
}

// TestParseScores_JSONWithExtraText tests parsing of JSON with surrounding text
func TestParseScores_JSONWithExtraText(t *testing.T) {
	scorer := &Scorer{}

	tests := []struct {
		name     string
		response string
		wantExp  float64
		wantErr  bool
	}{
		{
			name: "JSON with text before",
			response: `Here are the scores:
{
	"experience_score": 40.0,
	"experience_reasoning": "Good",
	"education_score": 15.0,
	"education_reasoning": "Adequate",
	"duties_score": 18.0,
	"duties_reasoning": "Strong",
	"cover_letter_score": 7.0,
	"cover_letter_reasoning": "Good"
}`,
			wantExp: 40.0,
			wantErr: false,
		},
		{
			name: "JSON with text after",
			response: `{
	"experience_score": 35.0,
	"experience_reasoning": "Fair",
	"education_score": 12.0,
	"education_reasoning": "Basic",
	"duties_score": 16.0,
	"duties_reasoning": "Acceptable",
	"cover_letter_score": 6.0,
	"cover_letter_reasoning": "Average"
}
Hope this helps!`,
			wantExp: 35.0,
			wantErr: false,
		},
		{
			name: "JSON with markdown code blocks",
			response: "```json\n" + `{
	"experience_score": 42.0,
	"experience_reasoning": "Very good",
	"education_score": 17.0,
	"education_reasoning": "Strong",
	"duties_score": 19.0,
	"duties_reasoning": "Excellent",
	"cover_letter_score": 9.0,
	"cover_letter_reasoning": "Outstanding"
}` + "\n```",
			wantExp: 42.0,
			wantErr: false,
		},
		{
			name:     "No JSON in response",
			response: "This response has no JSON object",
			wantErr:  true,
		},
		{
			name:     "Invalid JSON",
			response: "{ invalid json }",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scores, err := scorer.parseScores(tt.response)

			if tt.wantErr {
				if err == nil {
					t.Errorf("parseScores() expected error but got none")
				}
			} else {
				if err != nil {
					t.Fatalf("parseScores() failed: %v", err)
				}
				if scores.ExperienceScore != tt.wantExp {
					t.Errorf("ExperienceScore = %v, want %v", scores.ExperienceScore, tt.wantExp)
				}
			}
		})
	}
}
