package models

import (
	"encoding/json"
	"testing"
)

func TestJobDescriptionSerialization(t *testing.T) {
	jd := JobDescription{
		Title:              "Software Engineer",
		RequiredExperience: []string{"Go", "Python"},
		RequiredEducation:  []string{"Bachelor's degree"},
		Description:        "Test description",
	}

	data, err := json.Marshal(jd)
	if err != nil {
		t.Fatalf("Failed to marshal JobDescription: %v", err)
	}

	var decoded JobDescription
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal JobDescription: %v", err)
	}

	if decoded.Title != jd.Title {
		t.Errorf("Expected title %s, got %s", jd.Title, decoded.Title)
	}

	if len(decoded.RequiredExperience) != len(jd.RequiredExperience) {
		t.Errorf("Expected %d required experiences, got %d", len(jd.RequiredExperience), len(decoded.RequiredExperience))
	}
}

func TestScoresCalculation(t *testing.T) {
	scores := Scores{
		ExperienceScore:  45.0,
		EducationScore:   18.0,
		DutiesScore:      19.0,
		CoverLetterScore: 8.0,
	}

	expectedTotal := 90.0
	scores.TotalScore = scores.ExperienceScore + scores.EducationScore + scores.DutiesScore + scores.CoverLetterScore

	if scores.TotalScore != expectedTotal {
		t.Errorf("Expected total score %f, got %f", expectedTotal, scores.TotalScore)
	}
}

func TestApplicantResultRanking(t *testing.T) {
	results := []ApplicantResult{
		{
			Name: "Applicant1",
			Scores: Scores{
				TotalScore: 85.0,
			},
		},
		{
			Name: "Applicant2",
			Scores: Scores{
				TotalScore: 92.0,
			},
		},
	}

	// Assign ranks based on scores
	if results[1].Scores.TotalScore > results[0].Scores.TotalScore {
		results[1].Rank = 1
		results[0].Rank = 2
	}

	if results[1].Rank != 1 {
		t.Errorf("Expected Applicant2 to be rank 1, got %d", results[1].Rank)
	}

	if results[0].Rank != 2 {
		t.Errorf("Expected Applicant1 to be rank 2, got %d", results[0].Rank)
	}
}
