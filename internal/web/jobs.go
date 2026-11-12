package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/fmuoria/CV-Review-agent/internal/export"
	"github.com/fmuoria/CV-Review-agent/internal/posting"
	"github.com/fmuoria/CV-Review-agent/internal/store"
	"github.com/fmuoria/CV-Review-agent/internal/templates"
)

var countries = []struct{ Code, Name string }{{"KE", "Kenya"}, {"UG", "Uganda"}, {"RW", "Rwanda"}, {"MULTI", "Multiple / remote"}}

func (s *Server) jobsIndex(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	jobs, err := s.Store.ListJobs(r.Context(), u)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, "jobs", "Jobs", map[string]any{"Jobs": jobs, "Now": s.Now()})
}

type jobForm struct {
	Job       store.Job
	Templates []templates.RoleTemplate
	Template  string
	Countries any
	Managers  []store.User
	Members   map[int64]bool
	Errors    []string
	IsNew     bool
}

func (s *Server) jobFormData(r *http.Request, job store.Job, isNew bool) (jobForm, error) {
	users, err := s.Store.ListUsers(r.Context())
	if err != nil {
		return jobForm{}, err
	}
	var managers []store.User
	for _, u := range users {
		if u.Active && u.Role == store.RoleHiringManager {
			managers = append(managers, u)
		}
	}
	members := map[int64]bool{}
	if !isNew {
		if members, err = s.Store.JobMembers(r.Context(), job.ID); err != nil {
			return jobForm{}, err
		}
	}
	return jobForm{Job: job, Templates: templates.All(), Countries: countries, Managers: managers, Members: members, IsNew: isNew}, nil
}

func (s *Server) jobNew(w http.ResponseWriter, r *http.Request) {
	job := store.Job{Country: "KE", EmploymentType: "Full-time", Status: "draft", Location: "Nairobi, Kenya"}
	key := r.URL.Query().Get("template")
	if t, ok := templates.Get(key); ok && key != "general" {
		job.RoleFamily = t.Key
		job.Summary = t.Summary
		job.RequiredExperience, job.RequiredEducation, job.RequiredDuties = t.RequiredExperience, t.RequiredEducation, t.RequiredDuties
		job.NiceExperience, job.NiceEducation, job.NiceDuties = t.NiceExperience, t.NiceEducation, t.NiceDuties
		job.Questions = t.Questions
	}
	data, err := s.jobFormData(r, job, true)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	data.Template = key
	s.render(w, r, "job_form", "New job", data)
}

func parseJobForm(r *http.Request, job *store.Job) []string {
	var errs []string
	job.Title = strings.TrimSpace(r.FormValue("title"))
	job.Department = strings.TrimSpace(r.FormValue("department"))
	job.Location = strings.TrimSpace(r.FormValue("location"))
	job.Country = r.FormValue("country")
	job.EmploymentType = strings.TrimSpace(r.FormValue("employment_type"))
	job.RoleFamily = r.FormValue("role_family")
	if job.RoleFamily == "" {
		job.RoleFamily = "general"
	}
	job.Summary = strings.TrimSpace(r.FormValue("summary"))
	job.Description = strings.TrimSpace(r.FormValue("description"))
	job.RequiredExperience = splitLines(r.FormValue("required_experience"))
	job.RequiredEducation = splitLines(r.FormValue("required_education"))
	job.RequiredDuties = splitLines(r.FormValue("required_duties"))
	job.NiceExperience = splitLines(r.FormValue("nice_experience"))
	job.NiceEducation = splitLines(r.FormValue("nice_education"))
	job.NiceDuties = splitLines(r.FormValue("nice_duties"))
	job.Questions = splitLines(r.FormValue("questions"))
	job.ClosesOn = nil
	if d := strings.TrimSpace(r.FormValue("closes_on")); d != "" {
		t, err := time.Parse("2006-01-02", d)
		if err != nil {
			errs = append(errs, "Closing date must be a valid date.")
		} else {
			job.ClosesOn = &t
		}
	}
	if st := r.FormValue("status"); st == "draft" || st == "open" || st == "closed" {
		job.Status = st
	}
	if job.Title == "" {
		errs = append(errs, "Give the job a title.")
	}
	if len(job.RequiredExperience)+len(job.RequiredDuties) == 0 {
		errs = append(errs, "Add at least one required experience item or duty — the AI scores against these.")
	}
	if len(job.Questions) > 5 {
		errs = append(errs, "Keep extra application questions to five or fewer.")
	}
	return errs
}

func formIDs(r *http.Request, key string) []int64 {
	var ids []int64
	for _, v := range r.Form[key] {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

func (s *Server) jobCreate(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	job := store.Job{Status: "draft", CreatedBy: &u.ID}
	if errs := parseJobForm(r, &job); len(errs) > 0 {
		data, _ := s.jobFormData(r, job, true)
		data.Errors = errs
		s.renderStatus(w, r, http.StatusUnprocessableEntity, "job_form", "New job", data)
		return
	}
	if err := s.Store.SaveJob(r.Context(), &job); err != nil {
		s.serverError(w, r, err)
		return
	}
	if err := s.Store.SetJobMembers(r.Context(), job.ID, formIDs(r, "members")); err != nil {
		s.serverError(w, r, err)
		return
	}
	msg := "Job saved as a draft. Open it when you're ready to accept applications."
	if job.Status == "open" {
		msg = "Job is open and listed on the careers page."
	}
	s.redirect(w, r, fmt.Sprintf("/jobs/%d", job.ID), "ok", msg)
}

func (s *Server) jobEdit(w http.ResponseWriter, r *http.Request) {
	job, ok := s.loadJob(w, r)
	if !ok {
		return
	}
	data, err := s.jobFormData(r, job, false)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, "job_form", "Edit "+job.Title, data)
}

func (s *Server) jobUpdate(w http.ResponseWriter, r *http.Request) {
	job, ok := s.loadJob(w, r)
	if !ok {
		return
	}
	before := job.Spec()
	if errs := parseJobForm(r, &job); len(errs) > 0 {
		data, _ := s.jobFormData(r, job, false)
		data.Errors = errs
		s.renderStatus(w, r, http.StatusUnprocessableEntity, "job_form", "Edit "+job.Title, data)
		return
	}
	if err := s.Store.SaveJob(r.Context(), &job); err != nil {
		s.serverError(w, r, err)
		return
	}
	if err := s.Store.SetJobMembers(r.Context(), job.ID, formIDs(r, "members")); err != nil {
		s.serverError(w, r, err)
		return
	}
	msg := "Job updated."
	if !reflect.DeepEqual(before, job.Spec()) {
		msg = "Job updated. The requirements changed — use “Re-screen all” so existing scores reflect them."
	}
	s.redirect(w, r, fmt.Sprintf("/jobs/%d", job.ID), "ok", msg)
}

func (s *Server) jobSetStatus(w http.ResponseWriter, r *http.Request) {
	job, ok := s.loadJob(w, r)
	if !ok {
		return
	}
	st := r.FormValue("status")
	if st != "draft" && st != "open" && st != "closed" {
		s.redirect(w, r, fmt.Sprintf("/jobs/%d", job.ID), "error", "Unknown status.")
		return
	}
	if err := s.Store.SetJobStatus(r.Context(), job.ID, st); err != nil {
		s.serverError(w, r, err)
		return
	}
	msgs := map[string]string{"open": "Job is open and listed on the careers page.", "closed": "Job closed. It no longer accepts applications.", "draft": "Job moved back to draft."}
	s.redirect(w, r, fmt.Sprintf("/jobs/%d", job.ID), "ok", msgs[st])
}

func (s *Server) jobRescreen(w http.ResponseWriter, r *http.Request) {
	job, ok := s.loadJob(w, r)
	if !ok {
		return
	}
	u, _ := currentUser(r)
	n, err := s.Store.RequeueJob(r.Context(), job.ID, u.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.Screening.Wake()
	s.redirect(w, r, fmt.Sprintf("/jobs/%d", job.ID), "ok", fmt.Sprintf("%d application(s) queued for screening.", n))
}

func filterFromQuery(r *http.Request) store.ApplicationFilter {
	q := r.URL.Query()
	f := store.ApplicationFilter{Stage: q.Get("stage"), Source: q.Get("source"), Query: strings.TrimSpace(q.Get("q"))}
	if f.Stage == "" {
		f.Stage = "active"
	}
	if f.Stage == "all" {
		f.Stage = ""
	}
	if v, err := strconv.ParseFloat(q.Get("min"), 64); err == nil {
		f.MinScore = &v
	}
	return f
}

func (s *Server) jobShow(w http.ResponseWriter, r *http.Request) {
	job, ok := s.loadJob(w, r)
	if !ok {
		return
	}
	f := filterFromQuery(r)
	apps, err := s.Store.ListApplications(r.Context(), job.ID, f)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	for i := range apps {
		apps[i].Rank = i + 1
	}
	u, _ := currentUser(r)
	all, err := s.Store.ListJobs(r.Context(), u) // for counts
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	for _, j := range all {
		if j.ID == job.ID {
			job.StageCounts, job.Total, job.PendingCount = j.StageCounts, j.Total, j.PendingCount
		}
	}
	q := r.URL.Query()
	s.render(w, r, "job", job.Title, map[string]any{
		"Job": job, "Apps": apps, "Filter": f, "StageParam": q.Get("stage"), "Min": q.Get("min"),
		"PublicURL": s.Cfg.BaseURL + "/careers/" + job.Slug, "Open": job.IsOpen(s.Now()),
		"ExportQuery": r.URL.RawQuery,
	})
}

func (s *Server) jobStatusJSON(w http.ResponseWriter, r *http.Request) {
	job, ok := s.loadJob(w, r)
	if !ok {
		return
	}
	u, _ := currentUser(r)
	jobs, err := s.Store.ListJobs(r.Context(), u)
	if err != nil {
		http.Error(w, "error", 500)
		return
	}
	out := map[string]int{"pending": 0, "total": 0}
	for _, j := range jobs {
		if j.ID == job.ID {
			out["pending"], out["total"] = j.PendingCount, j.Total
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(out)
}

func (s *Server) jobPost(w http.ResponseWriter, r *http.Request) {
	job, ok := s.loadJob(w, r)
	if !ok {
		return
	}
	s.render(w, r, "job_post", "Post "+job.Title, map[string]any{
		"Job": job, "Copies": posting.Generate(s.Cfg.BaseURL, s.Cfg.OrgName, job), "Open": job.IsOpen(s.Now()),
	})
}

func (s *Server) jobExport(w http.ResponseWriter, r *http.Request) {
	job, ok := s.loadJob(w, r)
	if !ok {
		return
	}
	apps, err := s.Store.ListApplications(r.Context(), job.ID, filterFromQuery(r))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	u, _ := currentUser(r)
	_ = s.Store.AddEvent(r.Context(), 0, job.ID, &u.ID, "exported", map[string]any{"count": len(apps)})
	name := fmt.Sprintf("%s-candidates-%s.xlsx", job.Slug, s.Now().Format("2006-01-02"))
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	w.Header().Set("Cache-Control", "no-store")
	if err := export.JobReport(w, job, apps, s.Now()); err != nil {
		s.Log.Error("export", "err", err)
	}
}

func (s *Server) compare(w http.ResponseWriter, r *http.Request) {
	job, ok := s.loadJob(w, r)
	if !ok {
		return
	}
	var ids []int64
	for _, v := range strings.Split(r.URL.Query().Get("ids"), ",") {
		if id, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
			ids = append(ids, id)
		}
	}
	if len(ids) < 2 || len(ids) > 6 {
		s.redirect(w, r, fmt.Sprintf("/jobs/%d", job.ID), "error", "Select between 2 and 6 candidates to compare.")
		return
	}
	apps, err := s.Store.ListApplications(r.Context(), job.ID, store.ApplicationFilter{IDs: ids})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	// Build the requirement × candidate matrix from the required checks.
	type row struct {
		Requirement string
		Met         []string
	}
	var reqs []string
	for _, l := range [][]string{job.RequiredExperience, job.RequiredEducation, job.RequiredDuties} {
		reqs = append(reqs, l...)
	}
	rows := make([]row, len(reqs))
	for i, req := range reqs {
		rows[i] = row{Requirement: req, Met: make([]string, len(apps))}
		for j, a := range apps {
			rows[i].Met[j] = "—"
			if a.Scoring == nil {
				continue
			}
			for _, c := range a.Scoring.RequiredChecks {
				if strings.EqualFold(strings.TrimSpace(c.Requirement), strings.TrimSpace(req)) {
					rows[i].Met[j] = c.Met
				}
			}
		}
	}
	s.render(w, r, "compare", "Compare candidates", map[string]any{"Job": job, "Apps": apps, "Rows": rows})
}

// bulkAction handles the checkboxes on the ranked list.
func (s *Server) bulkAction(w http.ResponseWriter, r *http.Request) {
	job, ok := s.loadJob(w, r)
	if !ok {
		return
	}
	u, _ := currentUser(r)
	ids := formIDs(r, "ids")
	back := fmt.Sprintf("/jobs/%d", job.ID)
	if len(ids) == 0 {
		s.redirect(w, r, back, "error", "Select at least one candidate first.")
		return
	}
	// Only act on applications that belong to this job.
	apps, err := s.Store.ListApplications(r.Context(), job.ID, store.ApplicationFilter{IDs: ids})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	switch action := r.FormValue("action"); {
	case action == "compare":
		var parts []string
		for _, a := range apps {
			parts = append(parts, strconv.FormatInt(a.ID, 10))
		}
		http.Redirect(w, r, fmt.Sprintf("/jobs/%d/compare?ids=%s", job.ID, strings.Join(parts, ",")), http.StatusSeeOther)
	case action == "rescreen":
		for _, a := range apps {
			if err := s.Store.Requeue(r.Context(), a.ID, u.ID); err != nil {
				s.serverError(w, r, err)
				return
			}
		}
		s.Screening.Wake()
		s.redirect(w, r, back, "ok", fmt.Sprintf("%d application(s) queued for screening.", len(apps)))
	case strings.HasPrefix(action, "stage:"):
		stage := strings.TrimPrefix(action, "stage:")
		if !store.ValidStage(stage) {
			s.redirect(w, r, back, "error", "Unknown stage.")
			return
		}
		reason := strings.TrimSpace(r.FormValue("reason"))
		if stage == "rejected" && reason == "" {
			s.redirect(w, r, back, "error", "Add a short reason before rejecting candidates — it's kept in each candidate's history.")
			return
		}
		for _, a := range apps {
			if err := s.Store.SetStage(r.Context(), a.ID, stage, u.ID, reason); err != nil {
				s.serverError(w, r, err)
				return
			}
		}
		s.redirect(w, r, back, "ok", fmt.Sprintf("Moved %d candidate(s) to %s.", len(apps), store.StageLabels[stage]))
	default:
		s.redirect(w, r, back, "error", "Choose an action.")
	}
}
