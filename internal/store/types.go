// Package store reads and writes the ATS database.
package store

import (
	"errors"
	"time"

	"github.com/fmuoria/CV-Review-agent/internal/profile"
	"github.com/fmuoria/CV-Review-agent/internal/scoring"
)

// Roles.
const (
	RoleAdmin         = "admin"
	RoleHR            = "hr"
	RoleHiringManager = "hiring_manager"
)

// Roles lists every role with its display name.
var Roles = []struct{ Value, Label string }{
	{RoleAdmin, "Admin"},
	{RoleHR, "HR"},
	{RoleHiringManager, "Hiring manager"},
}

// Pipeline stages, in order.
var Stages = []string{"applied", "screened", "shortlisted", "interview", "offer", "hired", "rejected", "withdrawn"}

// StageLabels gives display names for stages.
var StageLabels = map[string]string{
	"applied": "Applied", "screened": "Screened", "shortlisted": "Shortlisted", "interview": "Interview",
	"offer": "Offer", "hired": "Hired", "rejected": "Rejected", "withdrawn": "Withdrawn",
}

// Sources describe where an applicant came from.
var Sources = []struct{ Value, Label string }{
	{"website", "Sinapis website"},
	{"linkedin", "LinkedIn"},
	{"brightermonday", "BrighterMonday"},
	{"email", "Email (Gmail import)"},
	{"referral", "Referral"},
	{"upload", "Uploaded by HR"},
	{"other", "Other"},
}

// SourceLabel returns the display name for a source value.
func SourceLabel(v string) string {
	for _, s := range Sources {
		if s.Value == v {
			return s.Label
		}
	}
	return v
}

// ValidSource reports whether v is a known source.
func ValidSource(v string) bool {
	for _, s := range Sources {
		if s.Value == v {
			return true
		}
	}
	return false
}

// ValidStage reports whether s is a pipeline stage.
func ValidStage(s string) bool {
	_, ok := StageLabels[s]
	return ok
}

// Errors.
var (
	ErrNotFound       = errors.New("not found")
	ErrAlreadyApplied = errors.New("this candidate has already applied for this job")
)

// User is a staff member who can sign in.
type User struct {
	ID          int64
	Email       string
	Name        string
	Role        string
	Active      bool
	CreatedAt   time.Time
	LastLoginAt *time.Time
}

// CanManageJobs reports whether the user can create and edit jobs (admin, HR).
func (u User) CanManageJobs() bool { return u.Role == RoleAdmin || u.Role == RoleHR }

// IsAdmin reports whether the user is an admin.
func (u User) IsAdmin() bool { return u.Role == RoleAdmin }

// DisplayName prefers the name, falling back to the email.
func (u User) DisplayName() string {
	if u.Name != "" {
		return u.Name
	}
	return u.Email
}

// Session is a signed-in browser session.
type Session struct {
	ID        string
	UserID    int64
	CSRFToken string
	ExpiresAt time.Time
}

// Job is a vacancy.
type Job struct {
	ID                 int64
	Slug               string
	Title              string
	Department         string
	Location           string
	Country            string
	EmploymentType     string
	RoleFamily         string
	Summary            string
	Description        string
	RequiredExperience []string
	RequiredEducation  []string
	RequiredDuties     []string
	NiceExperience     []string
	NiceEducation      []string
	NiceDuties         []string
	Questions          []string
	ClosesOn           *time.Time
	Status             string
	CreatedBy          *int64
	CreatedAt          time.Time
	UpdatedAt          time.Time

	// Filled by list queries.
	StageCounts  map[string]int
	Total        int
	PendingCount int
}

// IsOpen reports whether the job accepts applications today.
func (j Job) IsOpen(now time.Time) bool {
	if j.Status != "open" {
		return false
	}
	if j.ClosesOn != nil {
		closes := time.Date(j.ClosesOn.Year(), j.ClosesOn.Month(), j.ClosesOn.Day(), 23, 59, 59, 0, now.Location())
		return !now.After(closes)
	}
	return true
}

// Spec converts the job to the scorer's input format.
func (j Job) Spec() scoring.JobSpec {
	desc := j.Summary
	if j.Description != "" {
		desc += "\n" + j.Description
	}
	return scoring.JobSpec{
		Title: j.Title, Description: desc,
		RequiredExperience: j.RequiredExperience, RequiredEducation: j.RequiredEducation, RequiredDuties: j.RequiredDuties,
		NiceToHaveExperience: j.NiceExperience, NiceToHaveEducation: j.NiceEducation, NiceToHaveDuties: j.NiceDuties,
	}
}

// Candidate is a person, shared across applications.
type Candidate struct {
	ID          int64
	Email       string
	Name        string
	Phone       string
	Location    string
	LinkedInURL string
}

// Application links a candidate to a job and holds screening results.
type Application struct {
	ID          int64
	JobID       int64
	JobTitle    string
	CandidateID int64
	Candidate   Candidate
	Source      string
	SourceRef   string
	Stage       string
	Answers     []scoring.QA
	ConsentAt   *time.Time
	CoverText   string

	ScreeningStatus   string
	ScreeningError    string
	ScreeningAttempts int
	ScoreTotal        *float64
	ScoreExperience   *float64
	ScoreEducation    *float64
	ScoreDuties       *float64
	ScoreCover        *float64
	Scoring           *scoring.Result
	Profile           *profile.Profile
	Flags             []profile.Flag
	ScoredAt          *time.Time
	Model             string
	PromptVersion     string

	CreatedAt time.Time
	UpdatedAt time.Time

	Rank int // position in a ranked list (not stored)
}

// WarningCount counts warning-level flags.
func (a Application) WarningCount() int {
	n := 0
	for _, f := range a.Flags {
		if f.Severity == "warning" {
			n++
		}
	}
	return n
}

// Document is an uploaded file.
type Document struct {
	ID            int64
	ApplicationID int64
	Kind          string
	Filename      string
	ContentType   string
	SizeBytes     int64
	StorageKey    string
	TextContent   string
	Extraction    string
	CreatedAt     time.Time
}

// Event is an audit-log entry.
type Event struct {
	ID            int64
	ApplicationID *int64
	JobID         *int64
	ActorID       *int64
	ActorName     string
	Kind          string
	Detail        map[string]any
	CreatedAt     time.Time
}

// Note is a comment by a staff member on an application.
type Note struct {
	ID         int64
	AuthorName string
	Body       string
	CreatedAt  time.Time
}

// Integration is an organisation-wide connected account (e.g. Gmail).
type Integration struct {
	Name         string
	AccountEmail string
	SecretCipher string
	ConnectedAt  time.Time
}

// ApplicationFilter narrows a ranked list.
type ApplicationFilter struct {
	Stage    string
	Source   string
	MinScore *float64
	Query    string
	IDs      []int64
}
