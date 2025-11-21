package models

// JobDescription represents a job posting with requirements
type JobDescription struct {
	Title                string   `json:"title"`
	RequiredExperience   []string `json:"required_experience"`
	RequiredEducation    []string `json:"required_education"`
	RequiredDuties       []string `json:"required_duties"`
	NiceToHaveExperience []string `json:"nice_to_have_experience"`
	NiceToHaveEducation  []string `json:"nice_to_have_education"`
	NiceToHaveDuties     []string `json:"nice_to_have_duties"`
	Description          string   `json:"description"`
}

// ApplicantDocument holds CV and cover letter content
type ApplicantDocument struct {
	Name      string `json:"name"`
	CVContent string `json:"cv_content"`
	CVPath    string `json:"cv_path"`
	CLContent string `json:"cl_content"` // Cover Letter
	CLPath    string `json:"cl_path"`
}

// Scores represents evaluation scores for an applicant
type Scores struct {
	ExperienceScore      float64 `json:"experience_score"`   // 0-50
	EducationScore       float64 `json:"education_score"`    // 0-20
	DutiesScore          float64 `json:"duties_score"`       // 0-20
	CoverLetterScore     float64 `json:"cover_letter_score"` // 0-10
	TotalScore           float64 `json:"total_score"`        // 0-100
	ExperienceReasoning  string  `json:"experience_reasoning"`
	EducationReasoning   string  `json:"education_reasoning"`
	DutiesReasoning      string  `json:"duties_reasoning"`
	CoverLetterReasoning string  `json:"cover_letter_reasoning"`
}

// ApplicantResult represents the evaluation result for one applicant
type ApplicantResult struct {
	Name   string `json:"name"`
	Scores Scores `json:"scores"`
	Rank   int    `json:"rank"`
	CVPath string `json:"cv_path,omitempty"`
	CLPath string `json:"cl_path,omitempty"`
}

// IngestRequest represents the request payload for document ingestion
type IngestRequest struct {
	Method         string `json:"method"`          // "upload" or "gmail"
	GmailSubject   string `json:"gmail_subject"`   // Subject filter for Gmail
	JobDescription string `json:"job_description"` // Job description text
}

// ReportResponse represents the response with ranked applicants
type ReportResponse struct {
	Applicants []ApplicantResult `json:"applicants"`
	JobTitle   string            `json:"job_title"`
	Timestamp  string            `json:"timestamp"`
}
