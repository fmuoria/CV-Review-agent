package gmail

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func b64(s string) string { return base64.URLEncoding.EncodeToString([]byte(s)) }

// A forwarded email: the CV sits two levels deep, and the body is the cover letter.
func TestGetFindsNestedAttachmentsAndSender(t *testing.T) {
	msg := `{"id":"m1","payload":{"mimeType":"multipart/mixed","headers":[
		{"name":"From","value":"\"Kamau, John\" <John.Kamau@Example.com>"},{"name":"Subject","value":"Application: Programme Officer"}],
		"parts":[
		 {"mimeType":"multipart/alternative","parts":[{"mimeType":"text/plain","body":{"data":"` + b64("Dear Hiring Manager, please find my CV attached.") + `"}}]},
		 {"mimeType":"message/rfc822","parts":[{"mimeType":"multipart/mixed","parts":[
		    {"mimeType":"application/pdf","filename":"John_Kamau_CV.pdf","body":{"attachmentId":"a1","size":10}}]}]}
		]}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(401)
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/attachments/a1"):
			w.Write([]byte(`{"data":"` + b64("%PDF-1.4 cv") + `"}`))
		case strings.HasSuffix(r.URL.Path, "/messages/m1"):
			w.Write([]byte(msg))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c := &Client{Token: "tok", BaseURL: srv.URL}
	m, err := c.Get(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}
	if m.FromEmail != "john.kamau@example.com" || m.FromName != "Kamau, John" {
		t.Fatalf("sender = %q <%q>", m.FromName, m.FromEmail)
	}
	if len(m.Attachments) != 1 || m.Attachments[0].Filename != "John_Kamau_CV.pdf" || string(m.Attachments[0].Data) != "%PDF-1.4 cv" {
		t.Fatalf("attachments = %+v", m.Attachments)
	}
	if !strings.Contains(m.Body, "please find my CV") {
		t.Fatalf("body = %q", m.Body)
	}
}

func TestBuildQuery(t *testing.T) {
	q := BuildQuery("Programme Officer", time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC))
	if q != `has:attachment subject:("Programme Officer") after:2026/11/01` {
		t.Fatalf("query = %s", q)
	}
}
