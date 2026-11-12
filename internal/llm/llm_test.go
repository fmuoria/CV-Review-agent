package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fmuoria/CV-Review-agent/internal/googleauth"
)

func TestGeminiRequestAndRetry(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("missing bearer token")
		}
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		json.Unmarshal(body, &req)
		gc := req["generationConfig"].(map[string]any)
		if gc["responseMimeType"] != "application/json" {
			t.Errorf("JSON mode not requested")
		}
		if !strings.Contains(string(body), `"inlineData"`) {
			t.Errorf("PDF part not sent inline")
		}
		if n == 1 {
			w.WriteHeader(429)
			w.Write([]byte(`{"error":"rate limited"}`))
			return
		}
		w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"{\"ok\":true}"}]},"groundingMetadata":{"webSearchQueries":["q"],"groundingChunks":[{"web":{"uri":"https://e.org","title":"E"}}]}}]}`))
	}))
	defer srv.Close()

	c, err := NewGemini(GeminiConfig{Provider: "vertex", Project: "p", Tokens: googleauth.StaticToken("tok"),
		BaseURL: srv.URL, Backoff: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Generate(context.Background(), Request{JSON: true, Parts: []Part{{Text: "hi"}, {MIMEType: "application/pdf", Data: []byte("%PDF")}}})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("expected a retry after 429, calls=%d", calls)
	}
	var out struct{ OK bool }
	if err := DecodeJSON(resp.Text, &out); err != nil || !out.OK {
		t.Fatalf("decode: %v %+v", err, out)
	}
	if len(resp.Sources) != 1 || resp.SearchQueries[0] != "q" {
		t.Fatalf("grounding metadata not parsed: %+v", resp)
	}
}

func TestNonRetryableErrorStops(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(400)
	}))
	defer srv.Close()
	c, _ := NewGemini(GeminiConfig{Provider: "gemini", APIKey: "k", BaseURL: srv.URL, Backoff: time.Millisecond})
	if _, err := c.Generate(context.Background(), Request{Parts: []Part{{Text: "x"}}}); err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Fatalf("400 should not be retried, calls=%d", calls)
	}
}

func TestVertexEndpoint(t *testing.T) {
	c, _ := NewGemini(GeminiConfig{Provider: "vertex", Project: "sinapis-ats", Location: "europe-west1", Tokens: googleauth.StaticToken("t")})
	want := "https://europe-west1-aiplatform.googleapis.com/v1/projects/sinapis-ats/locations/europe-west1/publishers/google/models/gemini-2.5-flash:generateContent"
	if got := c.(*gemini).endpoint(); got != want {
		t.Fatalf("endpoint = %s", got)
	}
}
