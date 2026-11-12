package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fmuoria/CV-Review-agent/internal/gmail"
	"github.com/fmuoria/CV-Review-agent/internal/googleauth"
	"github.com/fmuoria/CV-Review-agent/internal/intake"
	"github.com/fmuoria/CV-Review-agent/internal/store"
)

// ---------------------------------------------------------------- bulk upload

func (s *Server) uploadPage(w http.ResponseWriter, r *http.Request) {
	job, ok := s.loadJob(w, r)
	if !ok {
		return
	}
	s.render(w, r, "job_upload", "Upload CVs", map[string]any{"Job": job, "MaxMB": s.Cfg.MaxUploadMB})
}

func (s *Server) uploadSubmit(w http.ResponseWriter, r *http.Request) {
	job, ok := s.loadJob(w, r)
	if !ok {
		return
	}
	back := fmt.Sprintf("/jobs/%d/upload", job.ID)
	if r.MultipartForm == nil {
		s.redirect(w, r, back, "error", "The upload was too large. Upload fewer files at a time (up to about 250 MB per batch).")
		return
	}
	source := r.FormValue("source")
	if !store.ValidSource(source) {
		source = "upload"
	}
	var files []intake.File
	var problems []string
	for _, fh := range r.MultipartForm.File["files"] {
		b, err := readUpload(fh)
		if err != nil {
			problems = append(problems, fh.Filename+": could not be read")
			continue
		}
		f := intake.File{Name: fh.Filename, Data: b}
		if err := s.Intake.Check(f); err != nil {
			problems = append(problems, err.Error())
			continue
		}
		files = append(files, f)
	}
	if len(files) == 0 {
		msg := "Choose at least one CV to upload."
		if len(problems) > 0 {
			msg = strings.Join(problems, " ")
		}
		s.redirect(w, r, back, "error", msg)
		return
	}
	added := 0
	for _, g := range intake.GroupUploads(files) {
		name := ""
		if strings.HasPrefix(g.Key, "name:") {
			name = splitCamel(strings.TrimPrefix(g.Key, "name:"), g.Files[0].Name)
		}
		_, err := s.Intake.Submit(r.Context(), store.NewApplication{JobID: job.ID, Name: name, Source: source}, g.Files)
		var ve *intake.ValidationError
		switch {
		case errors.As(err, &ve):
			problems = append(problems, g.Files[0].Name+": "+ve.Msg)
		case err != nil:
			problems = append(problems, g.Files[0].Name+": "+err.Error())
		default:
			added++
		}
	}
	msg := fmt.Sprintf("Added %d applicant(s). The AI is reading their CVs now — contact details are filled in from each CV.", added)
	kind := "ok"
	if len(problems) > 0 {
		msg += " Problems: " + strings.Join(problems, "; ")
		kind = "error"
	}
	s.redirect(w, r, fmt.Sprintf("/jobs/%d", job.ID), kind, msg)
}

// splitCamel turns "janesmith" (lower-cased key) back into "Jane Smith" using
// the original file name's capitalisation where possible.
func splitCamel(_ string, filename string) string {
	base := filename
	if i := strings.IndexAny(base, "_- "); i > 0 {
		base = base[:i]
	}
	var b strings.Builder
	for i, r := range base {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// ---------------------------------------------------------------- Gmail import

// importStatus tracks one running Gmail import (kept in memory).
type importStatus struct {
	Running    bool
	Query      string
	Found      int
	Processed  int
	Added      int
	Skipped    int
	Errors     []string
	StartedAt  time.Time
	FinishedAt time.Time
}

type importTracker struct {
	mu   sync.Mutex
	jobs map[int64]*importStatus
}

func newImportTracker() *importTracker { return &importTracker{jobs: map[int64]*importStatus{}} }

func (t *importTracker) get(jobID int64) (importStatus, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	st, ok := t.jobs[jobID]
	if !ok {
		return importStatus{}, false
	}
	cp := *st
	cp.Errors = append([]string(nil), st.Errors...)
	return cp, true
}

func (t *importTracker) update(jobID int64, fn func(*importStatus)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if st, ok := t.jobs[jobID]; ok {
		fn(st)
	}
}

func (t *importTracker) start(jobID int64, query string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if st, ok := t.jobs[jobID]; ok && st.Running {
		return false
	}
	t.jobs[jobID] = &importStatus{Running: true, Query: query, StartedAt: time.Now()}
	return true
}

func (s *Server) importPage(w http.ResponseWriter, r *http.Request) {
	job, ok := s.loadJob(w, r)
	if !ok {
		return
	}
	data := map[string]any{"Job": job, "DefaultSubject": job.Title, "DefaultSince": job.CreatedAt.Format("2006-01-02")}
	if integ, err := s.Store.GetIntegration(r.Context(), "gmail"); err == nil {
		data["Gmail"] = integ
	}
	if st, ok := s.imports.get(job.ID); ok {
		data["Status"] = st
	}
	s.render(w, r, "job_import", "Import from Gmail", data)
}

func (s *Server) importStart(w http.ResponseWriter, r *http.Request) {
	job, ok := s.loadJob(w, r)
	if !ok {
		return
	}
	back := fmt.Sprintf("/jobs/%d/import", job.ID)
	integ, err := s.Store.GetIntegration(r.Context(), "gmail")
	if err != nil {
		s.redirect(w, r, back, "error", "Connect a Gmail inbox in Settings first.")
		return
	}
	refresh, err := s.Box.Decrypt(integ.SecretCipher)
	if err != nil {
		s.redirect(w, r, back, "error", "The stored Gmail connection can't be read (was APP_SECRET changed?). Reconnect it in Settings.")
		return
	}
	var since time.Time
	if d := r.FormValue("since"); d != "" {
		since, _ = time.Parse("2006-01-02", d)
	}
	max, _ := strconv.Atoi(r.FormValue("max"))
	if max <= 0 || max > 1000 {
		max = 300
	}
	query := gmail.BuildQuery(r.FormValue("subject"), since)
	if !s.imports.start(job.ID, query) {
		s.redirect(w, r, back, "error", "An import for this job is already running.")
		return
	}
	u, _ := currentUser(r)
	go s.runGmailImport(job, refresh, query, max, u.ID)
	s.redirect(w, r, back, "ok", "Import started. This page refreshes until it finishes.")
}

func (s *Server) runGmailImport(job store.Job, refreshToken, query string, max int, actor int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()
	fail := func(msg string) {
		s.imports.update(job.ID, func(st *importStatus) {
			st.Errors = append(st.Errors, msg)
			st.Running, st.FinishedAt = false, time.Now()
		})
	}
	access, _, err := googleauth.RefreshAccessToken(ctx, s.Cfg.GoogleClientID, s.Cfg.GoogleClientSecret, refreshToken)
	if err != nil {
		fail("Gmail access was refused — reconnect the inbox in Settings. (" + err.Error() + ")")
		return
	}
	client := &gmail.Client{Token: access}
	ids, err := client.ListIDs(ctx, query, max)
	if err != nil {
		fail("Could not search Gmail: " + err.Error())
		return
	}
	s.imports.update(job.ID, func(st *importStatus) { st.Found = len(ids) })

	for _, id := range ids {
		ref := "gmail:" + id
		added, skipped, problem := s.importOne(ctx, client, job, id, ref)
		s.imports.update(job.ID, func(st *importStatus) {
			st.Processed++
			if added {
				st.Added++
			}
			if skipped {
				st.Skipped++
			}
			if problem != "" && len(st.Errors) < 50 {
				st.Errors = append(st.Errors, problem)
			}
		})
	}
	st, _ := s.imports.get(job.ID)
	_ = s.Store.AddEvent(ctx, 0, job.ID, &actor, "gmail_import", map[string]any{"query": query, "found": st.Found, "added": st.Added})
	s.imports.update(job.ID, func(st *importStatus) { st.Running, st.FinishedAt = false, time.Now() })
}

func (s *Server) importOne(ctx context.Context, client *gmail.Client, job store.Job, id, ref string) (added, skipped bool, problem string) {
	if exists, err := s.Store.SourceRefExists(ctx, job.ID, ref); err != nil {
		return false, false, err.Error()
	} else if exists {
		return false, true, ""
	}
	msg, err := client.Get(ctx, id)
	if err != nil {
		return false, false, "Could not read an email: " + err.Error()
	}
	var files []intake.File
	for _, a := range msg.Attachments {
		f := intake.File{Name: a.Filename, Data: a.Data}
		if s.Intake.Check(f) == nil {
			files = append(files, f)
		}
	}
	if len(files) == 0 {
		return false, true, ""
	}
	files = intake.AssignKinds(files)
	cover := ""
	if !hasKind(files, "cover_letter") && len(msg.Body) >= 200 {
		cover = msg.Body // the email itself is the cover letter
	}
	_, err = s.Intake.Submit(ctx, store.NewApplication{
		JobID: job.ID, Email: msg.FromEmail, Name: msg.FromName, Source: "email", SourceRef: ref, CoverText: cover,
	}, files)
	switch {
	case errors.Is(err, store.ErrAlreadyApplied):
		return false, true, ""
	case err != nil:
		return false, false, fmt.Sprintf("%s: %v", msg.FromEmail, err)
	}
	return true, false, ""
}
