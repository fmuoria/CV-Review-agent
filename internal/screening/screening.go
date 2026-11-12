// Package screening runs the AI review of new applications in the background.
//
// Applications are queued in the database (screening_status = 'pending').
// Workers claim one at a time, so the queue survives restarts and several
// workers never process the same application. For each application:
//
//  1. Extract text from the CV / cover letter (scanned PDFs go to Gemini).
//  2. Extract a structured profile and run the Go timeline checks.
//  3. Score against the job (the original rubric, with fixes).
//  4. Save scores, evidence, profile and flags.
//
// Stage changes are always left to people; screening never rejects anyone.
package screening

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/fmuoria/CV-Review-agent/internal/extract"
	"github.com/fmuoria/CV-Review-agent/internal/llm"
	"github.com/fmuoria/CV-Review-agent/internal/profile"
	"github.com/fmuoria/CV-Review-agent/internal/scoring"
	"github.com/fmuoria/CV-Review-agent/internal/storage"
	"github.com/fmuoria/CV-Review-agent/internal/store"
)

// MaxAttempts is how many times a failing application is retried.
const MaxAttempts = 3

// Service screens applications.
type Service struct {
	Store     *store.Store
	Files     storage.Store
	Extractor *extract.Extractor
	Scorer    *scoring.Scorer
	AI        llm.Client
	Log       *slog.Logger
	Now       func() time.Time
	Workers   int
	Delay     time.Duration // pause between AI calls per worker (rate limiting)
	Poll      time.Duration

	wake chan struct{}
	once sync.Once
}

func (s *Service) init() {
	s.once.Do(func() {
		s.wake = make(chan struct{}, 1)
		if s.Now == nil {
			s.Now = time.Now
		}
		if s.Workers <= 0 {
			s.Workers = 1
		}
		if s.Poll == 0 {
			s.Poll = 5 * time.Second
		}
		if s.Log == nil {
			s.Log = slog.Default()
		}
	})
}

// Wake nudges the workers to check the queue now (called after new applications).
func (s *Service) Wake() {
	s.init()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Run starts the workers and blocks until ctx is cancelled.
func (s *Service) Run(ctx context.Context) {
	s.init()
	if n, err := s.Store.RequeueStale(ctx, 15*time.Minute); err == nil && n > 0 {
		s.Log.Info("requeued interrupted screenings", "count", n)
	}
	var wg sync.WaitGroup
	for i := 0; i < s.Workers; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			s.loop(ctx, worker)
		}(i)
	}
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			return
		case <-ticker.C:
			_, _ = s.Store.RequeueStale(ctx, 15*time.Minute)
		}
	}
}

func (s *Service) loop(ctx context.Context, worker int) {
	for {
		if ctx.Err() != nil {
			return
		}
		id, err := s.Store.ClaimNextForScreening(ctx)
		if errors.Is(err, store.ErrNotFound) {
			select {
			case <-ctx.Done():
				return
			case <-s.wake:
			case <-time.After(s.Poll):
			}
			continue
		}
		if err != nil {
			s.Log.Error("claim screening", "err", err)
			sleep(ctx, s.Poll)
			continue
		}
		s.handle(ctx, id)
		sleep(ctx, s.Delay)
	}
}

func (s *Service) handle(ctx context.Context, id int64) {
	start := time.Now()
	err := s.Process(ctx, id)
	if err == nil {
		s.Log.Info("screened application", "application_id", id, "took", time.Since(start).Round(time.Millisecond))
		return
	}
	var unreadable *UnreadableError
	if errors.As(err, &unreadable) {
		_ = s.Store.FailScreening(ctx, id, unreadable.Error(), "needs_review")
		s.Log.Warn("application needs manual review", "application_id", id)
		return
	}
	app, getErr := s.Store.GetApplication(ctx, id)
	status := "failed"
	if getErr == nil && app.ScreeningAttempts < MaxAttempts {
		status = "pending" // try again on a later pass
	}
	_ = s.Store.FailScreening(ctx, id, err.Error(), status)
	s.Log.Error("screening failed", "application_id", id, "will_retry", status == "pending", "err", err)
}

// UnreadableError means the CV could not be read; a person must look at it.
type UnreadableError struct{ Reason string }

func (e *UnreadableError) Error() string { return e.Reason }

// Process screens one application synchronously.
func (s *Service) Process(ctx context.Context, appID int64) error {
	s.init()
	app, err := s.Store.GetApplication(ctx, appID)
	if err != nil {
		return err
	}
	job, err := s.Store.GetJob(ctx, app.JobID)
	if err != nil {
		return err
	}
	docs, err := s.Store.ListDocuments(ctx, appID)
	if err != nil {
		return err
	}

	var cvParts, coverParts []string
	if t := strings.TrimSpace(app.CoverText); t != "" {
		coverParts = append(coverParts, t)
	}
	for i := range docs {
		d := &docs[i]
		if d.TextContent == "" && d.Extraction != "failed" {
			if err := s.extractDoc(ctx, d); err != nil {
				return err
			}
		}
		if d.TextContent == "" {
			continue
		}
		switch d.Kind {
		case "cv":
			cvParts = append(cvParts, d.TextContent)
		case "cover_letter":
			coverParts = append(coverParts, d.TextContent)
		}
	}
	cvText := strings.Join(cvParts, "\n\n")
	if strings.TrimSpace(cvText) == "" {
		return &UnreadableError{"The CV could not be read automatically (missing, password-protected, or an unsupported format). Open the file and review it by hand, or ask the candidate for a PDF or Word version."}
	}

	now := s.Now()
	prof, err := profile.Extract(ctx, s.AI, cvText)
	if err != nil {
		return err
	}
	flags := prof.Analyse(now)
	if err := s.Store.FillCandidateFromProfile(ctx, app.CandidateID, prof); err != nil {
		s.Log.Warn("update candidate from profile", "application_id", appID, "err", err)
	}

	result, err := s.Scorer.Score(ctx, scoring.Input{
		Job:         job.Spec(),
		CVText:      cvText,
		CoverText:   strings.Join(coverParts, "\n\n"),
		Answers:     app.Answers,
		Timeline:    prof.TimelineSummary(now),
		TotalMonths: prof.TotalMonths,
	})
	if err != nil {
		return err
	}
	return s.Store.SaveScreening(ctx, appID, result, prof, flags, s.AI.Model())
}

func (s *Service) extractDoc(ctx context.Context, d *store.Document) error {
	data, err := s.Files.Get(ctx, d.StorageKey)
	if err != nil {
		return fmt.Errorf("load %s: %w", d.Kind, err)
	}
	text, method, err := s.Extractor.Text(ctx, d.Filename, data)
	if err != nil {
		if errors.Is(err, extract.ErrUnreadable) || strings.Contains(err.Error(), "unsupported") {
			_ = s.Store.SetDocumentText(ctx, d.ID, "", "failed")
			d.Extraction = "failed"
			return nil
		}
		return err // e.g. AI temporarily unavailable — retry later
	}
	d.TextContent, d.Extraction = text, method
	return s.Store.SetDocumentText(ctx, d.ID, text, method)
}

func sleep(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
