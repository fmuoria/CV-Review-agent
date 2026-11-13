package scoring

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/fmuoria/CV-Review-agent/internal/llm"
)

type fakeAI struct {
	answer string
	got    llm.Request
}

func (f *fakeAI) Generate(_ context.Context, r llm.Request) (llm.Response, error) {
	f.got = r
	return llm.Response{Text: f.answer}, nil
}
func (f *fakeAI) Model() string { return "fake" }

var job = JobSpec{
	Title: "Programme Officer",
	RequiredExperience: []string{
		"3+ years delivering entrepreneurship programmes",
		"Experience training SME owners",
		"Experience with monitoring and evaluation",
		"Experience managing partner relationships",
		"Experience with donor reporting",
	},
	RequiredEducation: []string{"Bachelor's degree in Business or related field"},
	RequiredDuties:    []string{"Deliver business training", "Track participant outcomes"},
}

// Fix 1: the prompt must use the real current date, not 22 November 2025.
func TestPromptUsesCurrentDate(t *testing.T) {
	now := time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
	p := BuildPrompt(Input{Job: job, CVText: "cv"}, now)
	if !strings.Contains(p, "12 November 2026") {
		t.Fatal("prompt does not contain today's date")
	}
	if strings.Contains(p, "2025-11-22") || strings.Contains(p, "November 22, 2025") {
		t.Fatal("prompt still contains the old hardcoded date")
	}
}

// Fix 3: every required item must reach the model (previously only the first 3).
func TestPromptIncludesAllRequiredItems(t *testing.T) {
	p := BuildPrompt(Input{Job: job, CVText: "cv"}, time.Now())
	for _, item := range append(append(job.RequiredExperience, job.RequiredEducation...), job.RequiredDuties...) {
		if !strings.Contains(p, item) {
			t.Errorf("required item missing from prompt: %q", item)
		}
	}
	if strings.Contains(p, "(+2 more)") {
		t.Error("required list was truncated")
	}
}

func TestPromptIncludesTimelineAndAnswers(t *testing.T) {
	p := BuildPrompt(Input{Job: job, CVText: "cv", Timeline: "TIMELINE-MARKER",
		Answers: []QA{{Question: "Why Sinapis?", Answer: "ANSWER-MARKER"}}}, time.Now())
	if !strings.Contains(p, "TIMELINE-MARKER") || !strings.Contains(p, "ANSWER-MARKER") {
		t.Fatal("timeline or answers missing from prompt")
	}
}

// Fix 2: out-of-range scores are clamped and the total recomputed.
func TestScoresAreClamped(t *testing.T) {
	ai := &fakeAI{answer: `{"experience_score": 65, "experience_reasoning": "x",
		"education_score": -5, "education_reasoning": "y",
		"duties_score": 25, "duties_reasoning": "z",
		"cover_letter_score": 12, "cover_letter_reasoning": "w", "total_score": 140}`}
	s := &Scorer{AI: ai, Now: time.Now}
	r, err := s.Score(context.Background(), Input{Job: job, CVText: "cv", CoverText: "letter"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Experience != 50 || r.Education != 0 || r.Duties != 20 || r.CoverLetter != 10 {
		t.Fatalf("not clamped: %+v", r)
	}
	if r.Total != 80 {
		t.Fatalf("total should be recomputed as 80, got %v", r.Total)
	}
	if !r.Clamped {
		t.Error("Clamped flag not set")
	}
	if r.Band != "Good match" {
		t.Errorf("band = %q", r.Band)
	}
}

func TestNoCoverLetterScoresZero(t *testing.T) {
	ai := &fakeAI{answer: `{"experience_score": 30, "experience_reasoning": "x", "education_score": 10,
		"education_reasoning": "y", "duties_score": 10, "duties_reasoning": "z", "cover_letter_score": 7}`}
	r, err := (&Scorer{AI: ai}).Score(context.Background(), Input{Job: job, CVText: "cv"})
	if err != nil {
		t.Fatal(err)
	}
	if r.CoverLetter != 0 || r.Total != 50 {
		t.Fatalf("cover letter should be 0 when none provided: %+v", r)
	}
}

func TestMarkdownFencedJSONIsParsed(t *testing.T) {
	ai := &fakeAI{answer: "```json\n{\"experience_score\": 40, \"experience_reasoning\": \"ok\", \"education_score\": 15, \"education_reasoning\": \"ok\", \"duties_score\": 15, \"duties_reasoning\": \"ok\", \"cover_letter_score\": 5, \"cover_letter_reasoning\": \"ok\", \"required_checks\": [{\"requirement\": \"a\", \"met\": \"YES\"}, {\"requirement\": \"b\", \"met\": \"maybe\"}]}\n```"}
	r, err := (&Scorer{AI: ai}).Score(context.Background(), Input{Job: job, CVText: "cv", CoverText: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Total != 75 {
		t.Fatalf("total = %v", r.Total)
	}
	if r.RequiredChecks[0].Met != "yes" || r.RequiredChecks[1].Met != "no" {
		t.Fatalf("met values not normalised: %+v", r.RequiredChecks)
	}
}

func TestEmptyScoresAreAnError(t *testing.T) {
	ai := &fakeAI{answer: `{}`}
	if _, err := (&Scorer{AI: ai}).Score(context.Background(), Input{Job: job, CVText: "cv"}); err == nil {
		t.Fatal("expected error for empty scores")
	}
}

func TestSanitizeUTF8(t *testing.T) {
	bad := "Jos\xe9 G\xf3mez"
	if got := sanitizeUTF8(bad); !utf8.ValidString(got) {
		t.Fatalf("still invalid: %q", got)
	}
	if got := sanitizeUTF8("José — 软件"); got != "José — 软件" {
		t.Fatalf("valid string changed: %q", got)
	}
}

func TestBands(t *testing.T) {
	for total, want := range map[float64]string{90: "Strong match", 85: "Strong match", 70: "Good match", 60: "Possible match", 20: "Weak match"} {
		if got := Band(total); got != want {
			t.Errorf("Band(%v) = %q, want %q", total, got, want)
		}
	}
}

func TestDemoHandler(t *testing.T) {
	out, err := DemoHandler(llm.Request{Hint: Input{Job: job, CVText: "Delivered entrepreneurship programmes and business training to SME owners; monitoring and evaluation.", TotalMonths: 48}})
	if err != nil {
		t.Fatal(err)
	}
	var r Result
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatal(err)
	}
	r.finalise()
	if r.Total <= 0 || r.Total > 100 {
		t.Fatalf("demo total out of range: %v", r.Total)
	}
	if len(r.RequiredChecks) != len(job.RequiredExperience)+len(job.RequiredEducation)+len(job.RequiredDuties) {
		t.Fatalf("expected a check per required item, got %d", len(r.RequiredChecks))
	}
}
