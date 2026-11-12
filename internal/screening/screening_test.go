package screening

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fmuoria/CV-Review-agent/internal/extract"
	"github.com/fmuoria/CV-Review-agent/internal/intake"
	"github.com/fmuoria/CV-Review-agent/internal/llm"
	"github.com/fmuoria/CV-Review-agent/internal/profile"
	"github.com/fmuoria/CV-Review-agent/internal/scoring"
	"github.com/fmuoria/CV-Review-agent/internal/storage"
	"github.com/fmuoria/CV-Review-agent/internal/store"
	"github.com/fmuoria/CV-Review-agent/internal/testdb"
)

func demoAI() *llm.Demo {
	d := llm.NewDemo()
	d.Handle("profile", profile.DemoHandler)
	d.Handle("score", scoring.DemoHandler)
	return d
}

func TestEndToEndScreening(t *testing.T) {
	conn := testdb.Open(t)
	ctx := context.Background()
	st := store.New(conn)
	files := storage.Local{Dir: t.TempDir()}

	hr, err := st.CreateUser(ctx, "hr@sinapis.org", "HR Person", store.RoleHR)
	if err != nil {
		t.Fatal(err)
	}
	job := store.Job{Title: "Senior Software Engineer", Status: "open", Country: "KE", CreatedBy: &hr.ID,
		RequiredExperience: []string{"5+ years of professional software development experience", "3+ years of Go programming experience"},
		RequiredEducation:  []string{"Bachelor's degree in Computer Science"},
		RequiredDuties:     []string{"Design and implement scalable backend services and APIs", "Conduct code reviews"}}
	if err := st.SaveJob(ctx, &job); err != nil {
		t.Fatal(err)
	}

	cv, err := os.ReadFile(filepath.Join("..", "..", "examples", "JohnDoe_CV.txt"))
	if err != nil {
		t.Fatal(err)
	}
	cl, _ := os.ReadFile(filepath.Join("..", "..", "examples", "JohnDoe_CoverLetter.txt"))
	in := &intake.Service{Store: st, Files: files, MaxBytes: 10 << 20}
	appID, err := in.Submit(ctx, store.NewApplication{JobID: job.ID, Email: "john.doe@example.com", Name: "John Doe", Source: "linkedin", Consent: true},
		[]intake.File{{Name: "JohnDoe_CV.txt", Data: cv, Kind: "cv"}, {Name: "JohnDoe_CoverLetter.txt", Data: cl, Kind: "cover_letter"}})
	if err != nil {
		t.Fatal(err)
	}
	// Duplicate application is rejected.
	if _, err := in.Submit(ctx, store.NewApplication{JobID: job.ID, Email: "John.Doe@example.com"}, []intake.File{{Name: "cv.txt", Data: cv, Kind: "cv"}}); err != store.ErrAlreadyApplied {
		t.Fatalf("expected ErrAlreadyApplied, got %v", err)
	}

	ai := demoAI()
	svc := &Service{Store: st, Files: files, Extractor: &extract.Extractor{AI: ai}, Scorer: &scoring.Scorer{AI: ai, Now: time.Now}, AI: ai}
	claimed, err := st.ClaimNextForScreening(ctx)
	if err != nil || claimed != appID {
		t.Fatalf("claim: %v %d", err, claimed)
	}
	if err := svc.Process(ctx, appID); err != nil {
		t.Fatal(err)
	}
	app, err := st.GetApplication(ctx, appID)
	if err != nil {
		t.Fatal(err)
	}
	if app.ScreeningStatus != "done" || app.ScoreTotal == nil || app.Scoring == nil || app.Profile == nil {
		t.Fatalf("not screened: %+v", app)
	}
	if *app.ScoreTotal <= 0 || *app.ScoreTotal > 100 {
		t.Fatalf("total out of range: %v", *app.ScoreTotal)
	}
	if app.Stage != "applied" {
		t.Fatalf("screening must not change the stage, got %s", app.Stage)
	}
	if app.Source != "linkedin" || app.ConsentAt == nil {
		t.Fatalf("source/consent not stored: %+v", app)
	}

	// Stage change is logged with the actor.
	if err := st.SetStage(ctx, appID, "shortlisted", hr.ID, ""); err != nil {
		t.Fatal(err)
	}
	events, _ := st.ListEvents(ctx, appID)
	kinds := map[string]bool{}
	for _, e := range events {
		kinds[e.Kind] = true
	}
	for _, k := range []string{"applied", "screened", "stage_changed"} {
		if !kinds[k] {
			t.Errorf("missing %s event", k)
		}
	}

	// A hiring manager not assigned to the job cannot see it.
	hm, _ := st.CreateUser(ctx, "hm@sinapis.org", "", store.RoleHiringManager)
	if ok, _ := st.CanAccessJob(ctx, hm, job.ID); ok {
		t.Fatal("unassigned hiring manager can access job")
	}
	st.SetJobMembers(ctx, job.ID, []int64{hm.ID})
	if ok, _ := st.CanAccessJob(ctx, hm, job.ID); !ok {
		t.Fatal("assigned hiring manager cannot access job")
	}
	jobs, _ := st.ListJobs(ctx, hm)
	if len(jobs) != 1 || jobs[0].Total != 1 || jobs[0].StageCounts["shortlisted"] != 1 {
		t.Fatalf("job counts wrong: %+v", jobs)
	}
}

func TestUnreadableCVNeedsReview(t *testing.T) {
	conn := testdb.Open(t)
	ctx := context.Background()
	st := store.New(conn)
	files := storage.Local{Dir: t.TempDir()}
	job := store.Job{Title: "Officer", Status: "open"}
	st.SaveJob(ctx, &job)
	in := &intake.Service{Store: st, Files: files}
	// A "PDF" with no text and no AI OCR handler available.
	appID, err := in.Submit(ctx, store.NewApplication{JobID: job.ID, Email: "x@y.org"}, []intake.File{{Name: "scan.pdf", Data: []byte("%PDF-1.4\n"), Kind: "cv"}})
	if err != nil {
		t.Fatal(err)
	}
	ai := demoAI() // no "transcribe" handler -> AI OCR errors
	svc := &Service{Store: st, Files: files, Extractor: &extract.Extractor{}, Scorer: scoring.New(ai), AI: ai}
	st.ClaimNextForScreening(ctx)
	svc.handle(ctx, appID)
	app, _ := st.GetApplication(ctx, appID)
	if app.ScreeningStatus != "needs_review" {
		t.Fatalf("status = %s (%s)", app.ScreeningStatus, app.ScreeningError)
	}
}

func TestProfileUpgradesPartialName(t *testing.T) {
	conn := testdb.Open(t)
	ctx := context.Background()
	st := store.New(conn)
	job := store.Job{Title: "Officer", Status: "open"}
	st.SaveJob(ctx, &job)
	id, err := st.CreateApplication(ctx, store.NewApplication{JobID: job.ID, Name: "Wanjiku"})
	if err != nil {
		t.Fatal(err)
	}
	app, _ := st.GetApplication(ctx, id)
	if err := st.FillCandidateFromProfile(ctx, app.CandidateID, profile.Profile{Name: "Wanjiku Mwangi", Email: "w@example.org"}); err != nil {
		t.Fatal(err)
	}
	app, _ = st.GetApplication(ctx, id)
	if app.Candidate.Name != "Wanjiku Mwangi" || app.Candidate.Email != "w@example.org" {
		t.Fatalf("candidate = %+v", app.Candidate)
	}
	// A different full name is never overwritten.
	st.FillCandidateFromProfile(ctx, app.CandidateID, profile.Profile{Name: "Someone Else"})
	app, _ = st.GetApplication(ctx, id)
	if app.Candidate.Name != "Wanjiku Mwangi" {
		t.Fatalf("name overwritten: %s", app.Candidate.Name)
	}
}
