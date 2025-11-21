package agent

import (
	"errors"
	"sort"
	"testing"

	"github.com/fmuoria/CV-Review-agent/internal/models"
)

// TestIsRateLimitError tests the rate limit error detection
func TestIsRateLimitError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "Nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "ResourceExhausted error",
			err:      errors.New("rpc error: code = ResourceExhausted desc = Resource exhausted"),
			expected: true,
		},
		{
			name:     "HTTP 429 error",
			err:      errors.New("HTTP 429: Too Many Requests"),
			expected: true,
		},
		{
			name:     "Rate limit error",
			err:      errors.New("rate limit exceeded"),
			expected: true,
		},
		{
			name:     "Quota error",
			err:      errors.New("quota exceeded for this project"),
			expected: true,
		},
		{
			name:     "Other error",
			err:      errors.New("connection timeout"),
			expected: false,
		},
		{
			name:     "Invalid JSON error",
			err:      errors.New("failed to parse JSON"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRateLimitError(tt.err)
			if result != tt.expected {
				t.Errorf("isRateLimitError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

// TestRateLimitConstants tests that rate limit constants are set correctly
func TestRateLimitConstants(t *testing.T) {
	// Verify that the constants are set to reasonable values
	if requestDelay.Seconds() != 4 {
		t.Errorf("requestDelay = %v, want 4 seconds", requestDelay)
	}

	if maxRetries != 3 {
		t.Errorf("maxRetries = %d, want 3", maxRetries)
	}

	if retryBackoff.Seconds() != 10 {
		t.Errorf("retryBackoff = %v, want 10 seconds", retryBackoff)
	}
}

// TestSortingWithTieBreaking tests the tie-breaking logic for equal total scores
func TestSortingWithTieBreaking(t *testing.T) {
	tests := []struct {
		name     string
		results  []models.ApplicantResult
		expected []string // Expected order of names
	}{
		{
			name: "Sort by total score (no ties)",
			results: []models.ApplicantResult{
				{Name: "Alice", Scores: models.Scores{TotalScore: 70}},
				{Name: "Bob", Scores: models.Scores{TotalScore: 90}},
				{Name: "Carol", Scores: models.Scores{TotalScore: 80}},
			},
			expected: []string{"Bob", "Carol", "Alice"},
		},
		{
			name: "Tie on total score, broken by experience score",
			results: []models.ApplicantResult{
				{Name: "Alice", Scores: models.Scores{TotalScore: 80, ExperienceScore: 40}},
				{Name: "Bob", Scores: models.Scores{TotalScore: 80, ExperienceScore: 45}},
				{Name: "Carol", Scores: models.Scores{TotalScore: 90, ExperienceScore: 35}},
			},
			expected: []string{"Carol", "Bob", "Alice"},
		},
		{
			name: "Tie on total and experience, broken by duties score",
			results: []models.ApplicantResult{
				{Name: "Alice", Scores: models.Scores{TotalScore: 80, ExperienceScore: 40, DutiesScore: 15}},
				{Name: "Bob", Scores: models.Scores{TotalScore: 80, ExperienceScore: 40, DutiesScore: 18}},
				{Name: "Carol", Scores: models.Scores{TotalScore: 90}},
			},
			expected: []string{"Carol", "Bob", "Alice"},
		},
		{
			name: "Tie on total, experience, and duties, broken by education score",
			results: []models.ApplicantResult{
				{Name: "Alice", Scores: models.Scores{TotalScore: 80, ExperienceScore: 40, DutiesScore: 15, EducationScore: 12}},
				{Name: "Bob", Scores: models.Scores{TotalScore: 80, ExperienceScore: 40, DutiesScore: 15, EducationScore: 18}},
			},
			expected: []string{"Bob", "Alice"},
		},
		{
			name: "Tie on all except cover letter, broken by cover letter score",
			results: []models.ApplicantResult{
				{Name: "Alice", Scores: models.Scores{TotalScore: 80, ExperienceScore: 40, DutiesScore: 15, EducationScore: 18, CoverLetterScore: 7}},
				{Name: "Bob", Scores: models.Scores{TotalScore: 80, ExperienceScore: 40, DutiesScore: 15, EducationScore: 18, CoverLetterScore: 9}},
			},
			expected: []string{"Bob", "Alice"},
		},
		{
			name: "Complete tie (all scores equal)",
			results: []models.ApplicantResult{
				{Name: "Alice", Scores: models.Scores{TotalScore: 80, ExperienceScore: 40, DutiesScore: 15, EducationScore: 18, CoverLetterScore: 7}},
				{Name: "Bob", Scores: models.Scores{TotalScore: 80, ExperienceScore: 40, DutiesScore: 15, EducationScore: 18, CoverLetterScore: 7}},
			},
			// Order doesn't matter for complete ties, just verify both are present
			expected: []string{"Alice", "Bob"}, // or ["Bob", "Alice"] is also acceptable
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy to avoid modifying the test case
			results := make([]models.ApplicantResult, len(tt.results))
			copy(results, tt.results)

			// Apply the same sorting logic as in processApplicants
			sort.Slice(results, func(i, j int) bool {
				// Primary: Total score
				if results[i].Scores.TotalScore != results[j].Scores.TotalScore {
					return results[i].Scores.TotalScore > results[j].Scores.TotalScore
				}

				// Tie-breaker 1: Experience score (most important for ranking)
				if results[i].Scores.ExperienceScore != results[j].Scores.ExperienceScore {
					return results[i].Scores.ExperienceScore > results[j].Scores.ExperienceScore
				}

				// Tie-breaker 2: Duties score (can they do the job?)
				if results[i].Scores.DutiesScore != results[j].Scores.DutiesScore {
					return results[i].Scores.DutiesScore > results[j].Scores.DutiesScore
				}

				// Tie-breaker 3: Education score
				if results[i].Scores.EducationScore != results[j].Scores.EducationScore {
					return results[i].Scores.EducationScore > results[j].Scores.EducationScore
				}

				// Tie-breaker 4: Cover letter score
				return results[i].Scores.CoverLetterScore > results[j].Scores.CoverLetterScore
			})

			// Check if the order matches expected (for complete ties, order can vary)
			isCompleteTie := tt.name == "Complete tie (all scores equal)"
			for i, name := range tt.expected {
				if !isCompleteTie && results[i].Name != name {
					t.Errorf("Position %d: got %s, want %s", i, results[i].Name, name)
				}
				if isCompleteTie {
					// Just verify all expected names are present
					found := false
					for _, r := range results {
						if r.Name == name {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("Expected name %s not found in results", name)
					}
				}
			}
		})
	}
}
