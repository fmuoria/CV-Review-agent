// Package web serves the HR dashboard and the public careers pages.
//
// Pages are rendered on the server with html/template (auto-escaping) and
// plain HTML forms; the only JavaScript is a small file for copy buttons and
// refreshing while screening runs. No front-end build step is needed.
package web

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/fmuoria/CV-Review-agent/internal/config"
	"github.com/fmuoria/CV-Review-agent/internal/intake"
	"github.com/fmuoria/CV-Review-agent/internal/profile"
	"github.com/fmuoria/CV-Review-agent/internal/screening"
	"github.com/fmuoria/CV-Review-agent/internal/secure"
	"github.com/fmuoria/CV-Review-agent/internal/storage"
	"github.com/fmuoria/CV-Review-agent/internal/store"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

// Server holds the dependencies of the web app.
type Server struct {
	Cfg       config.Config
	Store     *store.Store
	Files     storage.Store
	Intake    *intake.Service
	Screening *screening.Service
	Box       *secure.Box
	Log       *slog.Logger
	DemoAI    bool
	Now       func() time.Time

	pages   map[string]*template.Template
	imports *importTracker
}

type ctxKey int

const (
	ctxUser ctxKey = iota
	ctxSession
)

const sessionCookie = "ats_session"
const sessionTTL = 12 * time.Hour

// maxBulkUploadBytes caps one bulk-upload request (individual files are capped by MAX_UPLOAD_MB).
const maxBulkUploadBytes = 250 << 20

// Handler builds the HTTP handler with all routes.
func (s *Server) Handler() (http.Handler, error) {
	if s.Now == nil {
		s.Now = time.Now
	}
	if s.Log == nil {
		s.Log = slog.Default()
	}
	s.imports = newImportTracker()
	if s.Cfg.Location != nil {
		displayTZ = s.Cfg.Location
	}
	if err := s.loadTemplates(); err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	static, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", cacheStatic(http.FileServer(http.FS(static)))))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })

	// Public careers site.
	mux.HandleFunc("GET /careers", s.careersIndex)
	mux.HandleFunc("GET /careers/{slug}", s.careersJob)
	mux.HandleFunc("POST /careers/{slug}", s.careersApply)
	mux.HandleFunc("GET /careers/{slug}/thanks", s.careersThanks)

	// Sign-in.
	mux.HandleFunc("GET /login", s.loginPage)
	mux.HandleFunc("GET /auth/google", s.googleStart)
	mux.HandleFunc("GET /auth/callback", s.googleCallback)
	mux.HandleFunc("POST /dev/login", s.devLogin)
	mux.HandleFunc("POST /logout", s.logout)

	// Staff dashboard.
	auth := func(h http.HandlerFunc) http.Handler { return s.requireUser(h) }
	manage := func(h http.HandlerFunc) http.Handler { return s.requireUser(s.requireManager(h)) }
	admin := func(h http.HandlerFunc) http.Handler { return s.requireUser(s.requireAdmin(h)) }

	mux.Handle("GET /{$}", auth(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/jobs", http.StatusSeeOther) }))
	mux.Handle("GET /jobs", auth(s.jobsIndex))
	mux.Handle("GET /jobs/new", manage(s.jobNew))
	mux.Handle("POST /jobs", manage(s.jobCreate))
	mux.Handle("GET /jobs/{id}", auth(s.jobShow))
	mux.Handle("GET /jobs/{id}/status.json", auth(s.jobStatusJSON))
	mux.Handle("GET /jobs/{id}/edit", manage(s.jobEdit))
	mux.Handle("POST /jobs/{id}", manage(s.jobUpdate))
	mux.Handle("POST /jobs/{id}/status", manage(s.jobSetStatus))
	mux.Handle("POST /jobs/{id}/rescreen", manage(s.jobRescreen))
	mux.Handle("GET /jobs/{id}/post", manage(s.jobPost))
	mux.Handle("GET /jobs/{id}/upload", manage(s.uploadPage))
	mux.Handle("POST /jobs/{id}/upload", manage(s.uploadSubmit))
	mux.Handle("GET /jobs/{id}/import", manage(s.importPage))
	mux.Handle("POST /jobs/{id}/import", manage(s.importStart))
	mux.Handle("GET /jobs/{id}/export.xlsx", auth(s.jobExport))
	mux.Handle("GET /jobs/{id}/compare", auth(s.compare))
	mux.Handle("POST /jobs/{id}/bulk", auth(s.bulkAction))

	mux.Handle("GET /applications/{id}", auth(s.applicationShow))
	mux.Handle("POST /applications/{id}/stage", auth(s.applicationStage))
	mux.Handle("POST /applications/{id}/notes", auth(s.applicationNote))
	mux.Handle("POST /applications/{id}/rescreen", auth(s.applicationRescreen))
	mux.Handle("GET /documents/{id}", auth(s.documentDownload))

	mux.Handle("GET /admin/users", admin(s.usersPage))
	mux.Handle("POST /admin/users", admin(s.userCreate))
	mux.Handle("POST /admin/users/{id}", admin(s.userUpdate))
	mux.Handle("GET /settings", manage(s.settingsPage))
	mux.Handle("GET /settings/gmail/connect", manage(s.gmailConnect))
	mux.Handle("GET /settings/gmail/callback", manage(s.gmailCallback))
	mux.Handle("POST /settings/gmail/disconnect", manage(s.gmailDisconnect))

	var h http.Handler = mux
	h = s.withSession(h)
	h = s.securityHeaders(h)
	h = s.recoverer(h)
	h = s.logRequests(h)
	return h, nil
}

func cacheStatic(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=3600")
		h.ServeHTTP(w, r)
	})
}

// ---------------------------------------------------------------- middleware

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(rw, r)
		if strings.HasPrefix(r.URL.Path, "/static/") || r.URL.Path == "/healthz" {
			return
		}
		// Paths only — never log query strings or form bodies (they can contain candidate data).
		s.Log.Info("request", "method", r.Method, "path", r.URL.Path, "status", rw.status, "took", time.Since(start).Round(time.Millisecond))
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				s.Log.Error("panic", "path", r.URL.Path, "err", v)
				http.Error(w, "Something went wrong.", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; form-action 'self' https://accounts.google.com; base-uri 'self'")
		if strings.HasPrefix(r.URL.Path, "/careers") {
			// Allow the careers pages to be embedded on sinapis.org.
			h.Set("Content-Security-Policy", h.Get("Content-Security-Policy")+"; frame-ancestors 'self' https://*.sinapis.org https://sinapis.org")
		} else {
			h.Set("X-Frame-Options", "DENY")
			h.Set("Content-Security-Policy", h.Get("Content-Security-Policy")+"; frame-ancestors 'none'")
		}
		if s.Cfg.IsProduction() {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) withSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
			if sess, u, err := s.Store.GetSession(r.Context(), c.Value); err == nil {
				ctx := context.WithValue(r.Context(), ctxUser, u)
				ctx = context.WithValue(ctx, ctxSession, sess)
				r = r.WithContext(ctx)
			}
		}
		next.ServeHTTP(w, r)
	})
}

func currentUser(r *http.Request) (store.User, bool) {
	u, ok := r.Context().Value(ctxUser).(store.User)
	return u, ok
}

func currentSession(r *http.Request) (store.Session, bool) {
	s, ok := r.Context().Value(ctxSession).(store.Session)
	return s, ok
}

func (s *Server) requireUser(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := currentUser(r); !ok {
			http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
			return
		}
		if r.Method == http.MethodPost && strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			r.Body = http.MaxBytesReader(w, r.Body, maxBulkUploadBytes)
		}
		if r.Method == http.MethodPost && !s.validCSRF(r) {
			http.Error(w, "Your session form expired. Go back, refresh the page and try again.", http.StatusForbidden)
			return
		}
		next(w, r)
	})
}

func (s *Server) requireManager(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if u, _ := currentUser(r); !u.CanManageJobs() {
			s.forbidden(w, r)
			return
		}
		next(w, r)
	}
}

func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if u, _ := currentUser(r); !u.IsAdmin() {
			s.forbidden(w, r)
			return
		}
		next(w, r)
	}
}

func (s *Server) validCSRF(r *http.Request) bool {
	sess, ok := currentSession(r)
	if !ok {
		return false
	}
	token := r.Header.Get("X-CSRF-Token")
	if token == "" {
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			_ = r.ParseMultipartForm(32 << 20)
		}
		token = r.FormValue("csrf")
	}
	return token != "" && token == sess.CSRFToken
}

// ---------------------------------------------------------------- rendering

// displayTZ is the time zone dates are shown in; set from config in Handler.
var displayTZ = time.UTC

func local(t time.Time) time.Time { return t.In(displayTZ) }

var funcs = template.FuncMap{
	"stageLabel":  func(s string) string { return store.StageLabels[s] },
	"sourceLabel": store.SourceLabel,
	"score": func(p *float64) string {
		if p == nil {
			return "—"
		}
		return strconv.FormatFloat(*p, 'f', 1, 64)
	},
	"num": func(f float64) string { return strconv.FormatFloat(f, 'f', 1, 64) },
	"scoreClass": func(p *float64) string {
		if p == nil {
			return "score-none"
		}
		switch {
		case *p >= 70:
			return "score-high"
		case *p >= 55:
			return "score-mid"
		}
		return "score-low"
	},
	"pct": func(v *float64, max float64) int {
		if v == nil || max == 0 {
			return 0
		}
		return int(*v / max * 100)
	},
	"pctf": func(v, max float64) int {
		if max == 0 {
			return 0
		}
		return int(v / max * 100)
	},
	"date": func(t any) string {
		switch v := t.(type) {
		case time.Time:
			return local(v).Format("2 Jan 2006")
		case *time.Time:
			if v == nil {
				return ""
			}
			return local(*v).Format("2 Jan 2006")
		}
		return ""
	},
	"datetime": func(t time.Time) string { return local(t).Format("2 Jan 2006, 15:04") },
	"isodate": func(t *time.Time) string {
		if t == nil {
			return ""
		}
		return t.Format("2006-01-02")
	},
	"lines":  func(xs []string) string { return strings.Join(xs, "\n") },
	"months": profile.FormatMonths,
	"years": func(m int) string {
		return strconv.FormatFloat(float64(m)/12, 'f', 1, 64)
	},
	"add":   func(a, b int) int { return a + b },
	"lower": strings.ToLower,
	"roleLabel": func(r string) string {
		for _, x := range store.Roles {
			if x.Value == r {
				return x.Label
			}
		}
		return r
	},
	"statusLabel": func(s string) string {
		return map[string]string{"pending": "Waiting", "processing": "Screening…", "done": "Screened",
			"failed": "Failed", "needs_review": "Needs manual review"}[s]
	},
	"metIcon": func(m string) string {
		return map[string]string{"yes": "✓", "partial": "Partly", "no": "✗"}[m]
	},
	"eventText":  eventText,
	"stages":     func() []string { return store.Stages },
	"sources":    func() []struct{ Value, Label string } { return store.Sources },
	"roles":      func() []struct{ Value, Label string } { return store.Roles },
	"contains":   func(m map[int64]bool, id int64) bool { return m[id] },
	"deref":      func(b *bool) bool { return b != nil && *b },
	"stageCount": func(m map[string]int, s string) int { return m[s] },
	"safeURL": func(u string) template.URL {
		if strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://") {
			return template.URL(u)
		}
		if strings.Contains(u, "linkedin.com") {
			return template.URL("https://" + strings.TrimPrefix(u, "//"))
		}
		return ""
	},
}

func eventText(e store.Event) string {
	d := e.Detail
	switch e.Kind {
	case "applied":
		return fmt.Sprintf("Applied via %s", store.SourceLabel(fmt.Sprint(d["source"])))
	case "screened":
		return fmt.Sprintf("AI screening complete — %v/100 (%v)", d["total"], d["band"])
	case "screening_failed":
		return fmt.Sprintf("Screening stopped: %v", d["error"])
	case "stage_changed":
		t := fmt.Sprintf("Moved from %s to %s", store.StageLabels[fmt.Sprint(d["from"])], store.StageLabels[fmt.Sprint(d["to"])])
		if r, _ := d["reason"].(string); r != "" {
			t += " — " + r
		}
		return t
	case "note_added":
		return "Added a note"
	case "rescreen_requested":
		return "Asked for the application to be screened again"
	}
	return e.Kind
}

func (s *Server) loadTemplates() error {
	s.pages = map[string]*template.Template{}
	names, err := fs.Glob(templateFS, "templates/*.html")
	if err != nil {
		return err
	}
	for _, n := range names {
		base := strings.TrimSuffix(strings.TrimPrefix(n, "templates/"), ".html")
		if base == "layout" || base == "public" {
			continue
		}
		layout := "templates/layout.html"
		if strings.HasPrefix(base, "careers") {
			layout = "templates/public.html"
		}
		t, err := template.New(base).Funcs(funcs).ParseFS(templateFS, layout, n)
		if err != nil {
			return fmt.Errorf("template %s: %w", base, err)
		}
		s.pages[base] = t
	}
	return nil
}

// page is the data every template receives.
type page struct {
	Title     string
	User      store.User
	SignedIn  bool
	CSRF      string
	Flash     string
	FlashKind string
	DemoAI    bool
	OrgName   string
	Path      string
	Data      any
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, name, title string, data any) {
	s.renderStatus(w, r, http.StatusOK, name, title, data)
}

func (s *Server) renderStatus(w http.ResponseWriter, r *http.Request, status int, name, title string, data any) {
	t, ok := s.pages[name]
	if !ok {
		http.Error(w, "missing template "+name, 500)
		return
	}
	p := page{Title: title, Data: data, DemoAI: s.DemoAI, OrgName: s.Cfg.OrgName, Path: r.URL.Path}
	if u, ok := currentUser(r); ok {
		p.User, p.SignedIn = u, true
	}
	if sess, ok := currentSession(r); ok {
		p.CSRF = sess.CSRFToken
	}
	p.Flash, p.FlashKind = s.popFlash(w, r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	layout := "layout"
	if strings.HasPrefix(name, "careers") {
		layout = "public"
	}
	if err := t.ExecuteTemplate(w, layout, p); err != nil {
		s.Log.Error("render", "template", name, "err", err)
	}
}

func (s *Server) setFlash(w http.ResponseWriter, kind, msg string) {
	http.SetCookie(w, &http.Cookie{Name: "ats_flash", Value: s.Box.Sign(kind+":"+msg, time.Minute), Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: s.Cfg.IsProduction()})
}

func (s *Server) popFlash(w http.ResponseWriter, r *http.Request) (string, string) {
	c, err := r.Cookie("ats_flash")
	if err != nil {
		return "", ""
	}
	http.SetCookie(w, &http.Cookie{Name: "ats_flash", Value: "", Path: "/", MaxAge: -1})
	v, err := s.Box.Verify(c.Value)
	if err != nil {
		return "", ""
	}
	kind, msg, _ := strings.Cut(v, ":")
	return msg, kind
}

func (s *Server) redirect(w http.ResponseWriter, r *http.Request, to, kind, msg string) {
	if msg != "" {
		s.setFlash(w, kind, msg)
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	s.renderStatus(w, r, http.StatusNotFound, "error", "Not found", "That page doesn't exist, or you don't have access to it.")
}

func (s *Server) forbidden(w http.ResponseWriter, r *http.Request) {
	s.renderStatus(w, r, http.StatusForbidden, "error", "No access", "Your role doesn't allow this. Ask an admin if you need access.")
}

func (s *Server) serverError(w http.ResponseWriter, r *http.Request, err error) {
	s.Log.Error("handler error", "path", r.URL.Path, "err", err)
	s.renderStatus(w, r, http.StatusInternalServerError, "error", "Something went wrong", "Something went wrong on our side. Try again; if it keeps happening, tell the system admin.")
}

func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

// loadJob fetches a job the current user may access.
func (s *Server) loadJob(w http.ResponseWriter, r *http.Request) (store.Job, bool) {
	id, ok := pathID(r)
	if !ok {
		s.notFound(w, r)
		return store.Job{}, false
	}
	return s.loadJobByID(w, r, id)
}

func (s *Server) loadJobByID(w http.ResponseWriter, r *http.Request, id int64) (store.Job, bool) {
	u, _ := currentUser(r)
	job, err := s.Store.GetJob(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		s.notFound(w, r)
		return job, false
	}
	if err != nil {
		s.serverError(w, r, err)
		return job, false
	}
	if ok, err := s.Store.CanAccessJob(r.Context(), u, id); err != nil || !ok {
		s.notFound(w, r) // don't reveal that the job exists
		return job, false
	}
	return job, true
}

func splitLines(s string) []string {
	var out []string
	for _, l := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		l = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(l), "•-*·"))
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}
