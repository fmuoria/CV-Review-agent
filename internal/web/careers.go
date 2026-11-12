package web

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/fmuoria/CV-Review-agent/internal/intake"
	"github.com/fmuoria/CV-Review-agent/internal/scoring"
	"github.com/fmuoria/CV-Review-agent/internal/store"
)

func (s *Server) careersIndex(w http.ResponseWriter, r *http.Request) {
	jobs, err := s.Store.ListPublicJobs(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, "careers", "Careers at "+s.Cfg.OrgName, map[string]any{"Jobs": jobs, "Src": cleanSrc(r.URL.Query().Get("src"))})
}

func cleanSrc(src string) string {
	src = strings.ToLower(strings.TrimSpace(src))
	if store.ValidSource(src) && src != "upload" && src != "email" {
		return src
	}
	return "website"
}

type applyForm struct {
	Job          store.Job
	Values       map[string]string
	Answers      []string
	Errors       []string
	Token        string
	Src          string
	PrivacyEmail string
	MaxMB        int64
	Open         bool
}

func (s *Server) applyFormData(job store.Job, src string) applyForm {
	return applyForm{
		Job: job, Values: map[string]string{}, Answers: make([]string, len(job.Questions)),
		Token: s.Box.Sign(fmt.Sprintf("apply|%d|%d", job.ID, s.Now().Unix()), 6*time.Hour),
		Src:   src, PrivacyEmail: s.Cfg.PrivacyEmail, MaxMB: s.Cfg.MaxUploadMB, Open: job.IsOpen(s.Now()),
	}
}

func (s *Server) careersJob(w http.ResponseWriter, r *http.Request) {
	job, err := s.Store.GetJobBySlug(r.Context(), r.PathValue("slug"))
	if err != nil || job.Status == "draft" {
		s.renderStatus(w, r, http.StatusNotFound, "careers_closed", "Job not found", map[string]any{"Missing": true})
		return
	}
	s.render(w, r, "careers_job", job.Title+" — "+s.Cfg.OrgName, s.applyFormData(job, cleanSrc(r.URL.Query().Get("src"))))
}

func readUpload(fh *multipart.FileHeader) ([]byte, error) {
	f, err := fh.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func (s *Server) careersApply(w http.ResponseWriter, r *http.Request) {
	job, err := s.Store.GetJobBySlug(r.Context(), r.PathValue("slug"))
	if err != nil || job.Status == "draft" {
		s.renderStatus(w, r, http.StatusNotFound, "careers_closed", "Job not found", map[string]any{"Missing": true})
		return
	}
	if !job.IsOpen(s.Now()) {
		s.renderStatus(w, r, http.StatusGone, "careers_closed", "Applications closed", map[string]any{"Job": job})
		return
	}
	limit := s.Cfg.MaxUploadMB<<20*3 + 1<<20
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		data := s.applyFormData(job, cleanSrc(r.FormValue("src")))
		data.Errors = []string{fmt.Sprintf("The upload was too large. Each file must be under %d MB.", s.Cfg.MaxUploadMB)}
		s.renderStatus(w, r, http.StatusRequestEntityTooLarge, "careers_job", job.Title, data)
		return
	}

	data := s.applyFormData(job, cleanSrc(r.FormValue("src")))
	for _, k := range []string{"name", "email", "phone", "location", "linkedin_url", "cover_text"} {
		data.Values[k] = strings.TrimSpace(r.FormValue(k))
	}
	for i := range job.Questions {
		data.Answers[i] = strings.TrimSpace(r.FormValue("q" + strconv.Itoa(i)))
	}

	// Spam protection: hidden honeypot field and a signed, time-stamped form token.
	if r.FormValue("website_url") != "" || !s.validApplyToken(r.FormValue("token"), job.ID) {
		data.Errors = []string{"Your form expired. Please check your details and submit again."}
		s.renderStatus(w, r, http.StatusBadRequest, "careers_job", job.Title, data)
		return
	}

	var errs []string
	v := data.Values
	if v["name"] == "" {
		errs = append(errs, "Enter your full name.")
	}
	if addr, err := mail.ParseAddress(v["email"]); err != nil || addr.Address != v["email"] {
		errs = append(errs, "Enter a valid email address.")
	}
	if v["linkedin_url"] != "" && !strings.Contains(strings.ToLower(v["linkedin_url"]), "linkedin.com/") {
		errs = append(errs, "The LinkedIn link should look like https://www.linkedin.com/in/your-name.")
	}
	if r.FormValue("consent") != "yes" {
		errs = append(errs, "Please confirm you have read the privacy notice.")
	}
	for i, a := range data.Answers {
		if a == "" {
			errs = append(errs, fmt.Sprintf("Answer question %d.", i+1))
		}
		if len(a) > 4000 {
			errs = append(errs, fmt.Sprintf("Keep your answer to question %d under about 600 words.", i+1))
		}
	}

	var files []intake.File
	for field, kind := range map[string]string{"cv": "cv", "cover_file": "cover_letter", "linkedin_pdf": "linkedin_pdf"} {
		fhs := r.MultipartForm.File[field]
		if len(fhs) == 0 || fhs[0].Size == 0 {
			continue
		}
		b, err := readUpload(fhs[0])
		if err != nil {
			errs = append(errs, "We couldn't read one of your files. Please try again.")
			continue
		}
		f := intake.File{Name: fhs[0].Filename, Data: b, Kind: kind}
		if err := s.Intake.Check(f); err != nil {
			errs = append(errs, err.Error())
			continue
		}
		files = append(files, f)
	}
	if !hasKind(files, "cv") {
		errs = append(errs, "Attach your CV (PDF or Word).")
	}
	if len(errs) > 0 {
		data.Errors = append(errs, "For your security, please attach your files again.")
		s.renderStatus(w, r, http.StatusUnprocessableEntity, "careers_job", job.Title, data)
		return
	}

	var answers []scoring.QA
	for i, q := range job.Questions {
		answers = append(answers, scoring.QA{Question: q, Answer: data.Answers[i]})
	}
	_, err = s.Intake.Submit(r.Context(), store.NewApplication{
		JobID: job.ID, Email: v["email"], Name: v["name"], Phone: v["phone"], Location: v["location"],
		LinkedInURL: v["linkedin_url"], Source: data.Src, Answers: answers, CoverText: v["cover_text"], Consent: true,
	}, files)
	var ve *intake.ValidationError
	switch {
	case errors.Is(err, store.ErrAlreadyApplied):
		data.Errors = []string{"You've already applied for this role with this email address. We'll be in touch."}
		s.renderStatus(w, r, http.StatusConflict, "careers_job", job.Title, data)
		return
	case errors.As(err, &ve):
		data.Errors = []string{ve.Msg}
		s.renderStatus(w, r, http.StatusUnprocessableEntity, "careers_job", job.Title, data)
		return
	case err != nil:
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/careers/"+job.Slug+"/thanks", http.StatusSeeOther)
}

func hasKind(files []intake.File, kind string) bool {
	for _, f := range files {
		if f.Kind == kind {
			return true
		}
	}
	return false
}

func (s *Server) validApplyToken(token string, jobID int64) bool {
	v, err := s.Box.Verify(token)
	if err != nil {
		return false
	}
	parts := strings.Split(v, "|")
	if len(parts) != 3 || parts[0] != "apply" || parts[1] != strconv.FormatInt(jobID, 10) {
		return false
	}
	issued, err := strconv.ParseInt(parts[2], 10, 64)
	// Bots submit instantly; people take at least a few seconds.
	return err == nil && s.Now().Unix()-issued >= 3
}

func (s *Server) careersThanks(w http.ResponseWriter, r *http.Request) {
	job, err := s.Store.GetJobBySlug(r.Context(), r.PathValue("slug"))
	if err != nil {
		s.renderStatus(w, r, http.StatusNotFound, "careers_closed", "Job not found", map[string]any{"Missing": true})
		return
	}
	s.render(w, r, "careers_thanks", "Application received", map[string]any{"Job": job})
}
