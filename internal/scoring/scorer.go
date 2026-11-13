// Package scoring rates a CV (and cover letter) against a job description.
//
// This is the scoring engine from the original CV Review Agent, with the
// rubric unchanged — Experience 0-50, Education 0-20, Duties 0-20, Cover
// letter 0-10 — and these fixes:
//
//  1. The current date comes from the clock (it was hardcoded to 22 Nov 2025).
//  2. Every score is clamped to its range, and the total to 0-100, so a model
//     that adds "bonus" points can no longer push a candidate above 100.
//  3. ALL required items are sent to the model (it previously sent only the
//     first 3 of each list).
//  4. Durations come from the Go-computed timeline (see package profile)
//     instead of asking the model to do date arithmetic.
//  5. The model also returns per-requirement evidence, strengths and gaps,
//     which the dashboard shows next to the score.
//  6. Raw model responses (which contain candidate data) are not logged.
//
// The AI recommends; people decide. Scores only rank applicants for review.
package scoring

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/fmuoria/CV-Review-agent/internal/llm"
)

// PromptVersion is stored with every score so results can be traced to the prompt that produced them.
const PromptVersion = "2026-11-phase1"

// Score ranges.
const (
	MaxExperience  = 50.0
	MaxEducation   = 20.0
	MaxDuties      = 20.0
	MaxCoverLetter = 10.0
)

// Limits that keep prompts a sensible size.
const (
	maxCVChars    = 15000
	maxCoverChars = 5000
	maxNiceItems  = 10
)

// JobSpec is the part of a job description the scorer needs.
type JobSpec struct {
	Title                string
	Description          string
	RequiredExperience   []string
	RequiredEducation    []string
	RequiredDuties       []string
	NiceToHaveExperience []string
	NiceToHaveEducation  []string
	NiceToHaveDuties     []string
}

// QA is an answer to an extra application question.
type QA struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// Input is everything known about one applicant.
type Input struct {
	Job         JobSpec
	CVText      string
	CoverText   string
	Answers     []QA
	Timeline    string // profile.TimelineSummary output
	TotalMonths int
}

// Check records whether one required item is evidenced in the CV.
type Check struct {
	Requirement string `json:"requirement"`
	Category    string `json:"category"` // experience, education, duties
	Met         string `json:"met"`      // yes, partial, no
	Evidence    string `json:"evidence"` // short quote or paraphrase from the CV
}

// Result is a scored applicant.
type Result struct {
	Experience  float64 `json:"experience_score"`
	Education   float64 `json:"education_score"`
	Duties      float64 `json:"duties_score"`
	CoverLetter float64 `json:"cover_letter_score"`
	Total       float64 `json:"total_score"`

	ExperienceReasoning  string `json:"experience_reasoning"`
	EducationReasoning   string `json:"education_reasoning"`
	DutiesReasoning      string `json:"duties_reasoning"`
	CoverLetterReasoning string `json:"cover_letter_reasoning"`

	Summary        string   `json:"summary"`
	Strengths      []string `json:"strengths"`
	Gaps           []string `json:"gaps"`
	RequiredChecks []Check  `json:"required_checks"`

	Clamped bool   `json:"clamped,omitempty"` // true if the model returned an out-of-range score
	Band    string `json:"band"`
}

// Band labels describe fit for review — they are not hire/no-hire decisions.
func Band(total float64) string {
	switch {
	case total >= 85:
		return "Strong match"
	case total >= 70:
		return "Good match"
	case total >= 55:
		return "Possible match"
	}
	return "Weak match"
}

// Scorer calls the AI and post-processes its answer.
type Scorer struct {
	AI  llm.Client
	Now func() time.Time
}

// New returns a Scorer using the real clock.
func New(ai llm.Client) *Scorer { return &Scorer{AI: ai, Now: time.Now} }

// Score evaluates one applicant.
func (s *Scorer) Score(ctx context.Context, in Input) (Result, error) {
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	prompt := BuildPrompt(in, now)
	resp, err := s.AI.Generate(ctx, llm.Request{
		Task:   "score",
		System: "You are an expert HR analyst for a non-profit that supports entrepreneurs in East Africa. You evaluate applicants fairly, only on job-relevant evidence in their documents.",
		Parts:  []llm.Part{{Text: prompt}},
		JSON:   true,
		Hint:   in,
	})
	if err != nil {
		return Result{}, fmt.Errorf("scoring request failed: %w", err)
	}
	var r Result
	if err := llm.DecodeJSON(resp.Text, &r); err != nil {
		return Result{}, fmt.Errorf("could not parse scores: %w", err)
	}
	if isEmpty(r) {
		return Result{}, fmt.Errorf("model returned empty scores")
	}
	if strings.TrimSpace(in.CoverText) == "" && len(in.Answers) == 0 {
		r.CoverLetter = 0
		r.CoverLetterReasoning = "No cover letter was provided."
	}
	r.finalise()
	return r, nil
}

// finalise clamps every component and recomputes the total.
func (r *Result) finalise() {
	var clamped bool
	r.Experience, clamped = clamp(r.Experience, MaxExperience, clamped)
	r.Education, clamped = clamp(r.Education, MaxEducation, clamped)
	r.Duties, clamped = clamp(r.Duties, MaxDuties, clamped)
	r.CoverLetter, clamped = clamp(r.CoverLetter, MaxCoverLetter, clamped)
	r.Clamped = clamped
	r.Total = round1(r.Experience + r.Education + r.Duties + r.CoverLetter)
	r.Band = Band(r.Total)
	for i := range r.RequiredChecks {
		switch strings.ToLower(r.RequiredChecks[i].Met) {
		case "yes", "partial", "no":
			r.RequiredChecks[i].Met = strings.ToLower(r.RequiredChecks[i].Met)
		default:
			r.RequiredChecks[i].Met = "no"
		}
		r.RequiredChecks[i].Evidence = truncate(r.RequiredChecks[i].Evidence, 300)
	}
}

func clamp(v, max float64, already bool) (float64, bool) {
	switch {
	case math.IsNaN(v) || v < 0:
		return 0, true
	case v > max:
		return max, true
	}
	return round1(v), already
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

func isEmpty(r Result) bool {
	return r.Experience == 0 && r.Education == 0 && r.Duties == 0 && r.CoverLetter == 0 &&
		r.ExperienceReasoning == "" && r.EducationReasoning == "" && r.DutiesReasoning == ""
}

// sanitizeUTF8 replaces invalid UTF-8 so the request body is always valid JSON text.
func sanitizeUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	return strings.ToValidUTF8(s, "�")
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}

func listBlock(sb *strings.Builder, label string, items []string, limit int) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(sb, "%s:\n", label)
	for i, it := range items {
		if limit > 0 && i >= limit {
			fmt.Fprintf(sb, "  (+%d more)\n", len(items)-limit)
			break
		}
		fmt.Fprintf(sb, "  %d. %s\n", i+1, strings.TrimSpace(it))
	}
}

// BuildPrompt assembles the scoring prompt. Exported for tests and for
// reviewing the exact text sent to the model.
func BuildPrompt(in Input, now time.Time) string {
	j := in.Job
	today := now.Format("2 January 2006")
	var sb strings.Builder

	sb.WriteString("Analyse the following job and applicant and score the applicant.\n\n")

	sb.WriteString("## JOB DESCRIPTION\n")
	fmt.Fprintf(&sb, "Title: %s\n", j.Title)
	if d := strings.TrimSpace(j.Description); d != "" {
		fmt.Fprintf(&sb, "Description: %s\n", truncate(d, 1500))
	}
	sb.WriteString("\n### REQUIRED QUALIFICATIONS (must have — higher weight)\n")
	listBlock(&sb, "Required experience", j.RequiredExperience, 0)
	listBlock(&sb, "Required education", j.RequiredEducation, 0)
	listBlock(&sb, "Required duties", j.RequiredDuties, 0)
	sb.WriteString("\n### NICE TO HAVE (optional — lower weight)\n")
	listBlock(&sb, "Nice-to-have experience", j.NiceToHaveExperience, maxNiceItems)
	listBlock(&sb, "Nice-to-have education", j.NiceToHaveEducation, maxNiceItems)
	listBlock(&sb, "Nice-to-have duties", j.NiceToHaveDuties, maxNiceItems)

	sb.WriteString("\n## APPLICANT\n")
	if in.Timeline != "" {
		sb.WriteString("### EMPLOYMENT TIMELINE\n")
		sb.WriteString(in.Timeline)
		sb.WriteString("\n")
	}
	sb.WriteString("### CV CONTENT\n")
	sb.WriteString(truncate(sanitizeUTF8(in.CVText), maxCVChars))
	sb.WriteString("\n\n")
	if c := strings.TrimSpace(in.CoverText); c != "" {
		sb.WriteString("### COVER LETTER\n")
		sb.WriteString(truncate(sanitizeUTF8(c), maxCoverChars))
		sb.WriteString("\n\n")
	}
	if len(in.Answers) > 0 {
		sb.WriteString("### ANSWERS TO APPLICATION QUESTIONS (assess together with the cover letter)\n")
		for _, qa := range in.Answers {
			fmt.Fprintf(&sb, "Q: %s\nA: %s\n", qa.Question, truncate(sanitizeUTF8(qa.Answer), 1500))
		}
		sb.WriteString("\n")
	}

	fmt.Fprintf(&sb, "## SCORING INSTRUCTIONS\n\nToday's date: %s.\n\n", today)

	sb.WriteString(`### 1. DATES AND DURATION
Use the EMPLOYMENT TIMELINE above for start dates, end dates and durations — it was calculated by the system and is correct.
Only if a role is missing from the timeline, read its dates from the CV; "Present"/"Current" means today.

### 2. READING THE CV
- Scan the ENTIRE document: sections can be in any order, in columns, tables, headers or footers.
- Look for skills in a skills section AND inside duties, achievements, education, certifications and projects.
- Accept synonyms and equivalent tools or methods. Weight hands-on experience above theoretical knowledge.
- Look for depth (years, scale, results), not just mentions.
- DO NOT guess or infer information that is not present. If something is missing, say so.
- Ignore personal characteristics (age, gender, religion, ethnicity, marital status, photos, health, nationality).
  They are not job-relevant and must not affect any score.

### 3. JOB TITLE RELEVANCE
`)
	fmt.Fprintf(&sb, "Target title: %q.\n", j.Title)
	sb.WriteString(`Match the applicant's previous titles to the target role:
- STRONG: exact or near-exact title, synonyms, other seniority levels of the same role, or titles doing the SAME core functions.
- MODERATE: adjacent roles in the same field that perform SOME of the required duties (duties must show it, not just the title).
- WEAK: same industry but different function, or transferable skills in a different domain.
- NONE: unrelated industry and function.
Job title alone is insufficient — cross-check with the duties listed under each role:
- matching title + matching duties → valid, score on duration
- matching title but different duties → max 15/50 for experience
- different title but matching duties → treat as MODERATE
A keyword in the wrong context is not experience (e.g. "collaborated with the finance team" is not finance experience).

### 4. QUANTIFIED ACHIEVEMENTS
Find numbers in the CV (people trained, businesses supported, budgets managed, % growth, portfolio size) and compare
them with the scale the job implies. Matching or exceeding achievements justify scoring at the TOP of the relevant band.
They never take a score above its maximum.

### 5. EXPERIENCE (0-50)
First check relevance (title + duties). If no relevant role exists → maximum 10, regardless of years.
Then, for RELEVANT experience only, by total duration:
- 0-6 months: 18-24 | 6-12 months: 24-28 | 1-2 years: 28-34 | 2-3 years: 34-40 | 3-5 years: 40-45 | 5+ years: 45-50
Missing a REQUIRED experience item is a major deduction; missing a nice-to-have is a minor one.

### 6. EDUCATION (0-20)
`)
	if len(j.RequiredEducation) > 0 {
		sb.WriteString(`Education IS required for this role:
- has ALL required education: 18-20 | MOST: 12-17 | SOME: 8-11 | missing required education: 0-7
`)
	} else {
		sb.WriteString(`No specific education is required — prioritise experience:
- relevant degree/diploma: 15-20 | any higher education: 10-14 | secondary school + strong experience: 8-12 | secondary only: 5-7
`)
	}
	if len(j.NiceToHaveEducation) > 0 {
		sb.WriteString("Nice-to-have education can lift the score within the range (never above 20).\n")
	}

	sb.WriteString("\n### 7. DUTIES (0-20)\n")
	if n := len(j.RequiredDuties); n > 0 {
		fmt.Fprintf(&sb, "There are %d required duties, worth about %.1f points each. For each one:\n", n, MaxDuties/float64(n))
	} else {
		sb.WriteString("No duties were listed; judge from the job description. For each core duty:\n")
	}
	sb.WriteString(`- strong evidence (several examples): full points | moderate (one example): 60-80% | weak (implied): 30-50% | none: 0
Nice-to-have duties can lift the score within the range (never above 20).

### 8. COVER LETTER AND ANSWERS (0-10)
Judge relevance to THIS role, specific examples, motivation for the organisation's mission, and clarity.
A generic letter that could be sent to any employer scores 0-4. If no cover letter or answers were provided, score 0.

### 9. REQUIRED CHECKS
For EVERY item under REQUIRED QUALIFICATIONS, record whether the CV shows it: "yes", "partial" or "no",
with a short piece of evidence quoted or closely paraphrased from the CV (empty if "no").

## OUTPUT
Return ONLY valid JSON:
{
  "experience_score": <0-50>,
  "experience_reasoning": "<1-2 sentences>",
  "education_score": <0-20>,
  "education_reasoning": "<1-2 sentences>",
  "duties_score": <0-20>,
  "duties_reasoning": "<1-2 sentences>",
  "cover_letter_score": <0-10>,
  "cover_letter_reasoning": "<1-2 sentences>",
  "summary": "<2-3 sentence overall assessment for the hiring team>",
  "strengths": ["<up to 4 short points>"],
  "gaps": ["<up to 4 short points — things to probe at interview>"],
  "required_checks": [{"requirement": "<item text>", "category": "experience|education|duties", "met": "yes|partial|no", "evidence": "<short>"}]
}
`)
	return sb.String()
}

// --- Demo mode -------------------------------------------------------------

var reWord = regexp.MustCompile(`[a-z][a-z+#\-]{3,}`)

var stop = map[string]bool{
	"with": true, "from": true, "that": true, "this": true, "have": true, "experience": true, "years": true,
	"ability": true, "strong": true, "work": true, "working": true, "related": true, "field": true, "including": true,
	"least": true, "proven": true, "track": true, "record": true, "knowledge": true, "skills": true, "good": true,
	"excellent": true, "demonstrated": true, "relevant": true, "degree": true, "other": true, "such": true, "their": true,
}

func words(s string) []string {
	var out []string
	for _, w := range reWord.FindAllString(strings.ToLower(s), -1) {
		if !stop[w] {
			out = append(out, w)
		}
	}
	return out
}

func coverage(item, corpus string) float64 {
	ws := words(item)
	if len(ws) == 0 {
		return 0
	}
	hit := 0
	for _, w := range ws {
		if strings.Contains(corpus, w) {
			hit++
		}
	}
	return float64(hit) / float64(len(ws))
}

// DemoHandler produces keyword-overlap scores so the app can be tried without
// an AI account. It is deliberately crude; never use it for real decisions.
func DemoHandler(req llm.Request) (string, error) {
	in, ok := req.Hint.(Input)
	if !ok {
		return "", fmt.Errorf("demo scorer needs an Input hint")
	}
	cv := strings.ToLower(in.CVText)
	var checks []Check
	avg := func(cat string, items []string) float64 {
		if len(items) == 0 {
			return 0.7
		}
		total := 0.0
		for _, it := range items {
			c := coverage(it, cv)
			met := "no"
			switch {
			case c >= 0.6:
				met = "yes"
			case c >= 0.3:
				met = "partial"
			}
			checks = append(checks, Check{Requirement: it, Category: cat, Met: met, Evidence: map[bool]string{true: "Keyword match in CV (demo mode)", false: ""}[met != "no"]})
			total += c
		}
		return total / float64(len(items))
	}
	exp := avg("experience", in.Job.RequiredExperience)
	edu := avg("education", in.Job.RequiredEducation)
	dut := avg("duties", in.Job.RequiredDuties)
	tenure := math.Min(1, float64(in.TotalMonths)/60)
	cover := 0.0
	if strings.TrimSpace(in.CoverText) != "" {
		cover = 4 + 6*coverage(in.Job.Title+" "+strings.Join(in.Job.RequiredDuties, " "), strings.ToLower(in.CoverText))
	}
	r := Result{
		Experience:           10 + 40*exp*(0.5+0.5*tenure),
		Education:            20 * edu,
		Duties:               20 * dut,
		CoverLetter:          cover,
		ExperienceReasoning:  "Demo mode: keyword overlap with required experience, adjusted for time in employment.",
		EducationReasoning:   "Demo mode: keyword overlap with required education.",
		DutiesReasoning:      "Demo mode: keyword overlap with required duties.",
		CoverLetterReasoning: "Demo mode: overlap between the cover letter and the role.",
		Summary:              "Demo-mode score based on keyword matching only. Connect Gemini for a real assessment.",
		RequiredChecks:       checks,
	}
	b, err := json.Marshal(r)
	return string(b), err
}
