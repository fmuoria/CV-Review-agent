package web

import (
	"errors"
	"fmt"
	"mime"
	"net/http"
	"strings"

	"github.com/fmuoria/CV-Review-agent/internal/storage"
	"github.com/fmuoria/CV-Review-agent/internal/store"
)

func (s *Server) loadApp(w http.ResponseWriter, r *http.Request) (store.Application, bool) {
	id, ok := pathID(r)
	if !ok {
		s.notFound(w, r)
		return store.Application{}, false
	}
	app, err := s.Store.GetApplication(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		s.notFound(w, r)
		return app, false
	}
	if err != nil {
		s.serverError(w, r, err)
		return app, false
	}
	u, _ := currentUser(r)
	if ok, err := s.Store.CanAccessJob(r.Context(), u, app.JobID); err != nil || !ok {
		s.notFound(w, r)
		return app, false
	}
	return app, true
}

func (s *Server) applicationShow(w http.ResponseWriter, r *http.Request) {
	app, ok := s.loadApp(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	docs, err := s.Store.ListDocuments(ctx, app.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	notes, err := s.Store.ListNotes(ctx, app.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	events, err := s.Store.ListEvents(ctx, app.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	job, err := s.Store.GetJob(ctx, app.JobID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	// Previous / next in the job's ranked list, for quick review.
	var prev, next int64
	if list, err := s.Store.ListApplications(ctx, app.JobID, store.ApplicationFilter{Stage: "active"}); err == nil {
		for i, a := range list {
			if a.ID == app.ID {
				if i > 0 {
					prev = list[i-1].ID
				}
				if i < len(list)-1 {
					next = list[i+1].ID
				}
			}
		}
	}
	name := app.Candidate.Name
	if name == "" {
		name = "Unnamed applicant"
	}
	s.render(w, r, "application", name, map[string]any{
		"App": app, "Job": job, "Docs": docs, "Notes": notes, "Events": events, "Prev": prev, "Next": next, "Name": name,
	})
}

func (s *Server) applicationStage(w http.ResponseWriter, r *http.Request) {
	app, ok := s.loadApp(w, r)
	if !ok {
		return
	}
	u, _ := currentUser(r)
	stage := r.FormValue("stage")
	reason := strings.TrimSpace(r.FormValue("reason"))
	back := fmt.Sprintf("/applications/%d", app.ID)
	if !store.ValidStage(stage) {
		s.redirect(w, r, back, "error", "Unknown stage.")
		return
	}
	if stage == "rejected" && reason == "" {
		s.redirect(w, r, back, "error", "Add a short reason when rejecting — it's kept in the candidate's history.")
		return
	}
	if err := s.Store.SetStage(r.Context(), app.ID, stage, u.ID, reason); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.redirect(w, r, back, "ok", "Moved to "+store.StageLabels[stage]+".")
}

func (s *Server) applicationNote(w http.ResponseWriter, r *http.Request) {
	app, ok := s.loadApp(w, r)
	if !ok {
		return
	}
	u, _ := currentUser(r)
	body := strings.TrimSpace(r.FormValue("body"))
	back := fmt.Sprintf("/applications/%d#notes", app.ID)
	if body == "" {
		s.redirect(w, r, back, "error", "The note is empty.")
		return
	}
	if len(body) > 5000 {
		body = body[:5000]
	}
	if err := s.Store.AddNote(r.Context(), app.ID, u.ID, body); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.redirect(w, r, back, "ok", "Note added.")
}

func (s *Server) applicationRescreen(w http.ResponseWriter, r *http.Request) {
	app, ok := s.loadApp(w, r)
	if !ok {
		return
	}
	u, _ := currentUser(r)
	if err := s.Store.Requeue(r.Context(), app.ID, u.ID); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.Screening.Wake()
	s.redirect(w, r, fmt.Sprintf("/applications/%d", app.ID), "ok", "Queued for screening. Refresh in a minute.")
}

func (s *Server) documentDownload(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		s.notFound(w, r)
		return
	}
	doc, err := s.Store.GetDocument(r.Context(), id)
	if err != nil {
		s.notFound(w, r)
		return
	}
	app, err := s.Store.GetApplication(r.Context(), doc.ApplicationID)
	if err != nil {
		s.notFound(w, r)
		return
	}
	u, _ := currentUser(r)
	if ok, err := s.Store.CanAccessJob(r.Context(), u, app.JobID); err != nil || !ok {
		s.notFound(w, r)
		return
	}
	data, err := s.Files.Get(r.Context(), doc.StorageKey)
	if errors.Is(err, storage.ErrNotFound) {
		s.renderStatus(w, r, http.StatusNotFound, "error", "File missing", "The file record exists but the file itself was not found in storage.")
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	disposition := "attachment"
	if r.URL.Query().Get("inline") == "1" && doc.ContentType == "application/pdf" {
		disposition = "inline"
	}
	w.Header().Set("Content-Type", doc.ContentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": doc.Filename}))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Write(data)
}
