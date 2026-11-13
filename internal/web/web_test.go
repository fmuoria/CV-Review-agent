package web

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fmuoria/CV-Review-agent/internal/config"
	"github.com/fmuoria/CV-Review-agent/internal/extract"
	"github.com/fmuoria/CV-Review-agent/internal/intake"
	"github.com/fmuoria/CV-Review-agent/internal/llm"
	"github.com/fmuoria/CV-Review-agent/internal/profile"
	"github.com/fmuoria/CV-Review-agent/internal/scoring"
	"github.com/fmuoria/CV-Review-agent/internal/screening"
	"github.com/fmuoria/CV-Review-agent/internal/secure"
	"github.com/fmuoria/CV-Review-agent/internal/storage"
	"github.com/fmuoria/CV-Review-agent/internal/store"
	"github.com/fmuoria/CV-Review-agent/internal/testdb"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time      { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) Add(d time.Duration) { c.mu.Lock(); c.t = c.t.Add(d); c.mu.Unlock() }

type harness struct {
	t      *testing.T
	srv    *httptest.Server
	st     *store.Store
	screen *screening.Service
	clock  *clock
}

func newHarness(t *testing.T) *harness {
	conn := testdb.Open(t)
	st := store.New(conn)
	files := storage.Local{Dir: t.TempDir()}
	ai := llm.NewDemo()
	ai.Handle("profile", profile.DemoHandler)
	ai.Handle("score", scoring.DemoHandler)
	clk := &clock{t: time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)}
	screen := &screening.Service{Store: st, Files: files, Extractor: &extract.Extractor{AI: ai}, Scorer: &scoring.Scorer{AI: ai, Now: clk.Now}, AI: ai, Now: clk.Now}
	cfg := config.Config{Env: "development", BaseURL: "http://example.test", OrgName: "Sinapis", MaxUploadMB: 5, Location: time.UTC}
	s := &Server{Cfg: cfg, Store: st, Files: files, Screening: screen, Box: secure.New("test-secret-test-secret-test-secret"),
		Intake: &intake.Service{Store: st, Files: files, MaxBytes: 5 << 20}, DemoAI: true, Now: clk.Now}
	h, err := s.Handler()
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &harness{t: t, srv: srv, st: st, screen: screen, clock: clk}
}

func (h *harness) client() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar}
}

func body(t *testing.T, r *http.Response) string {
	t.Helper()
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return string(b)
}

var reCSRF = regexp.MustCompile(`name="csrf" value="([^"]+)"`)
var reToken = regexp.MustCompile(`name="token" value="([^"]+)"`)

func (h *harness) login(c *http.Client, email, role string) {
	r, err := c.PostForm(h.srv.URL+"/dev/login", url.Values{"email": {email}, "role": {role}, "next": {"/jobs"}})
	if err != nil || r.StatusCode != 200 {
		h.t.Fatalf("login: %v %v", err, r.StatusCode)
	}
	r.Body.Close()
}

func (h *harness) csrf(c *http.Client, path string) string {
	r, _ := c.Get(h.srv.URL + path)
	m := reCSRF.FindStringSubmatch(body(h.t, r))
	if m == nil {
		h.t.Fatalf("no csrf token on %s", path)
	}
	return m[1]
}

func TestHiringFlow(t *testing.T) {
	h := newHarness(t)
	hr := h.client()
	h.login(hr, "hr@sinapis.org", "hr")

	// Create and open a job.
	form := url.Values{"csrf": {h.csrf(hr, "/jobs/new")}, "title": {"MEL Officer"}, "country": {"KE"}, "status": {"open"},
		"required_experience": {"3+ years in monitoring and evaluation"}, "required_duties": {"Design data collection tools\nAnalyse programme data"},
		"questions": {"Why Sinapis?"}}
	r, _ := hr.PostForm(h.srv.URL+"/jobs", form)
	if page := body(t, r); !strings.Contains(page, "Job is open") {
		t.Fatalf("job not created: %s", page[:500])
	}
	jobPath := r.Request.URL.Path

	// Public application.
	pub := h.client()
	r, _ = pub.Get(h.srv.URL + "/careers/mel-officer?src=brightermonday")
	token := reToken.FindStringSubmatch(body(t, r))[1]
	h.clock.Add(10 * time.Second)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range map[string]string{"token": token, "src": "brightermonday", "name": "Achieng Otieno", "email": "achieng@example.org",
		"consent": "yes", "q0": "I love data for impact.", "cover_text": "I designed M&E systems for SME programmes."} {
		mw.WriteField(k, v)
	}
	fw, _ := mw.CreateFormFile("cv", "Achieng_CV.txt")
	fw.Write([]byte("Achieng Otieno\nachieng@example.org\nM&E Officer, Acme Foundation  Jan 2020 - Present\nDesigned data collection tools and analysed programme data for monitoring and evaluation.\n" + strings.Repeat("Experience with surveys and dashboards. ", 10)))
	mw.Close()
	r, _ = pub.Post(h.srv.URL+"/careers/mel-officer", mw.FormDataContentType(), &buf)
	if page := body(t, r); !strings.Contains(page, "received your application") {
		t.Fatalf("apply failed (%d): %s", r.StatusCode, page)
	}

	// Screen synchronously.
	id, err := h.st.ClaimNextForScreening(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := h.screen.Process(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	r, _ = hr.Get(h.srv.URL + jobPath)
	page := body(t, r)
	if !strings.Contains(page, "Achieng Otieno") || !strings.Contains(page, "BrighterMonday") || !strings.Contains(page, `class="score score-`) {
		t.Fatalf("ranked list missing applicant or score")
	}

	// Export works.
	r, _ = hr.Get(h.srv.URL + jobPath + "/export.xlsx")
	if b := body(t, r); r.StatusCode != 200 || !strings.HasPrefix(b, "PK") {
		t.Fatalf("export failed: %d", r.StatusCode)
	}

	// An unassigned hiring manager sees nothing.
	hm := h.client()
	h.login(hm, "hm@sinapis.org", "hiring_manager")
	if r, _ := hm.Get(h.srv.URL + jobPath); r.StatusCode != 404 {
		t.Fatalf("hiring manager got %d", r.StatusCode)
	}
	if r, _ := hm.Get(h.srv.URL + "/admin/users"); r.StatusCode != 403 {
		t.Fatalf("hiring manager reached admin page: %d", r.StatusCode)
	}

	// POST without CSRF is refused.
	if r, _ := hr.PostForm(h.srv.URL+jobPath+"/status", url.Values{"status": {"closed"}}); r.StatusCode != 403 {
		t.Fatalf("missing CSRF accepted: %d", r.StatusCode)
	}
}

func TestClosedJobRejectsApplications(t *testing.T) {
	h := newHarness(t)
	past := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	job := store.Job{Title: "Old Role", Status: "open", ClosesOn: &past, RequiredDuties: []string{"x"}}
	if err := h.st.SaveJob(context.Background(), &job); err != nil {
		t.Fatal(err)
	}
	r, _ := h.client().PostForm(h.srv.URL+"/careers/"+job.Slug, url.Values{"name": {"x"}})
	if r.StatusCode != http.StatusGone {
		t.Fatalf("closed job accepted post: %d", r.StatusCode)
	}
}
