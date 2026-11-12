// Package llm is a small Gemini client.
//
// It calls the Gemini REST API directly (no SDK), which replaces the
// cloud.google.com/go/vertexai/genai package that Google deprecated in 2025.
// Two backends are supported:
//
//   - Vertex AI (LLM_PROVIDER=vertex): billed to a Google Cloud project,
//     authenticated with a service account. Recommended for Sinapis.
//   - Gemini API / AI Studio (LLM_PROVIDER=gemini): API key. Use a PAID key —
//     free-tier prompts may be used by Google to improve its products, which
//     is not acceptable for candidate CVs.
//
// A deterministic demo client (LLM_PROVIDER=demo) lets you run the whole app
// locally with no Google account at all.
package llm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/fmuoria/CV-Review-agent/internal/googleauth"
)

// Part is one piece of prompt content: text, or a file such as a PDF.
type Part struct {
	Text     string
	MIMEType string
	Data     []byte
}

// Request is a single generation call.
type Request struct {
	System       string
	Parts        []Part
	JSON         bool    // ask for application/json output
	Temperature  float64 // 0 = use default 0.2
	MaxTokens    int     // 0 = use default 8192
	GoogleSearch bool    // ground the answer with Google Search (Phase 2 background checks)

	// Task names the kind of call ("profile", "score", "transcribe"). The real
	// Gemini client ignores it; the demo client uses it to pick a handler.
	Task string
	// Hint carries structured inputs for the demo client only.
	Hint any
}

// Source is a web page used to ground an answer.
type Source struct {
	Title string
	URI   string
}

// Response is the model's answer.
type Response struct {
	Text          string
	Sources       []Source
	SearchQueries []string
}

// Client generates content.
type Client interface {
	Generate(ctx context.Context, req Request) (Response, error)
	// Model names the model, recorded next to every score for auditability.
	Model() string
}

// APIError is a non-200 response from Gemini.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("gemini API error %d: %s", e.Status, truncate(e.Body, 300))
}

// Retryable reports whether the call may succeed if tried again later.
func (e *APIError) Retryable() bool {
	return e.Status == http.StatusTooManyRequests || e.Status >= 500
}

// GeminiConfig configures a real Gemini client.
type GeminiConfig struct {
	Provider string // "vertex" or "gemini"
	Model    string
	Project  string
	Location string
	APIKey   string
	Tokens   googleauth.TokenSource // required for vertex
	HTTP     *http.Client
	BaseURL  string // override for tests
	// Retry policy for 429 / 5xx responses.
	MaxAttempts int
	Backoff     time.Duration
}

type gemini struct{ cfg GeminiConfig }

// NewGemini returns a Client for Vertex AI or the Gemini API.
func NewGemini(cfg GeminiConfig) (Client, error) {
	if cfg.Model == "" {
		cfg.Model = "gemini-2.5-flash"
	}
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{Timeout: 3 * time.Minute}
	}
	if cfg.MaxAttempts == 0 {
		cfg.MaxAttempts = 4
	}
	if cfg.Backoff == 0 {
		cfg.Backoff = 5 * time.Second
	}
	switch cfg.Provider {
	case "vertex":
		if cfg.Project == "" || cfg.Tokens == nil {
			return nil, errors.New("vertex provider needs a project and credentials")
		}
		if cfg.Location == "" {
			cfg.Location = "us-central1"
		}
	case "gemini":
		if cfg.APIKey == "" {
			return nil, errors.New("gemini provider needs an API key")
		}
	default:
		return nil, fmt.Errorf("unknown provider %q", cfg.Provider)
	}
	return &gemini{cfg: cfg}, nil
}

func (g *gemini) Model() string { return g.cfg.Model }

func (g *gemini) endpoint() string {
	if g.cfg.BaseURL != "" {
		return strings.TrimRight(g.cfg.BaseURL, "/") + "/models/" + g.cfg.Model + ":generateContent"
	}
	if g.cfg.Provider == "gemini" {
		return "https://generativelanguage.googleapis.com/v1beta/models/" + g.cfg.Model + ":generateContent"
	}
	host := g.cfg.Location + "-aiplatform.googleapis.com"
	if g.cfg.Location == "global" {
		host = "aiplatform.googleapis.com"
	}
	return fmt.Sprintf("https://%s/v1/projects/%s/locations/%s/publishers/google/models/%s:generateContent",
		host, g.cfg.Project, g.cfg.Location, g.cfg.Model)
}

// Wire types for the generateContent REST API.
type wirePart struct {
	Text       string      `json:"text,omitempty"`
	InlineData *wireInline `json:"inlineData,omitempty"`
}
type wireInline struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}
type wireContent struct {
	Role  string     `json:"role,omitempty"`
	Parts []wirePart `json:"parts"`
}
type wireRequest struct {
	Contents          []wireContent    `json:"contents"`
	SystemInstruction *wireContent     `json:"systemInstruction,omitempty"`
	GenerationConfig  map[string]any   `json:"generationConfig"`
	Tools             []map[string]any `json:"tools,omitempty"`
}
type wireResponse struct {
	Candidates []struct {
		Content      wireContent `json:"content"`
		FinishReason string      `json:"finishReason"`
		Grounding    *struct {
			WebSearchQueries []string `json:"webSearchQueries"`
			GroundingChunks  []struct {
				Web *struct {
					URI   string `json:"uri"`
					Title string `json:"title"`
				} `json:"web"`
			} `json:"groundingChunks"`
		} `json:"groundingMetadata"`
	} `json:"candidates"`
	PromptFeedback *struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
}

func (g *gemini) Generate(ctx context.Context, req Request) (Response, error) {
	body, err := json.Marshal(buildWireRequest(req))
	if err != nil {
		return Response{}, err
	}

	var lastErr error
	for attempt := 1; attempt <= g.cfg.MaxAttempts; attempt++ {
		resp, err := g.call(ctx, body)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		var apiErr *APIError
		if !errors.As(err, &apiErr) || !apiErr.Retryable() || attempt == g.cfg.MaxAttempts {
			break
		}
		// Exponential backoff: 5s, 10s, 20s (the same policy the desktop agent used).
		wait := g.cfg.Backoff * time.Duration(1<<(attempt-1))
		select {
		case <-ctx.Done():
			return Response{}, ctx.Err()
		case <-time.After(wait):
		}
	}
	return Response{}, lastErr
}

func buildWireRequest(req Request) wireRequest {
	parts := make([]wirePart, 0, len(req.Parts))
	for _, p := range req.Parts {
		if len(p.Data) > 0 {
			parts = append(parts, wirePart{InlineData: &wireInline{
				MimeType: p.MIMEType, Data: base64.StdEncoding.EncodeToString(p.Data),
			}})
			continue
		}
		parts = append(parts, wirePart{Text: p.Text})
	}
	temp := req.Temperature
	if temp == 0 {
		temp = 0.2 // low temperature for consistent scoring
	}
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 8192
	}
	cfg := map[string]any{"temperature": temp, "maxOutputTokens": maxTokens}
	if req.JSON {
		cfg["responseMimeType"] = "application/json"
	}
	wr := wireRequest{
		Contents:         []wireContent{{Role: "user", Parts: parts}},
		GenerationConfig: cfg,
	}
	if req.System != "" {
		wr.SystemInstruction = &wireContent{Parts: []wirePart{{Text: req.System}}}
	}
	if req.GoogleSearch {
		wr.Tools = []map[string]any{{"googleSearch": map[string]any{}}}
	}
	return wr
}

func (g *gemini) call(ctx context.Context, body []byte) (Response, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, g.endpoint(), bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if g.cfg.Provider == "gemini" {
		httpReq.Header.Set("x-goog-api-key", g.cfg.APIKey)
	} else {
		tok, err := g.cfg.Tokens.Token(ctx)
		if err != nil {
			return Response{}, fmt.Errorf("google credentials: %w", err)
		}
		httpReq.Header.Set("Authorization", "Bearer "+tok)
	}

	httpResp, err := g.cfg.HTTP.Do(httpReq)
	if err != nil {
		return Response{}, &APIError{Status: 503, Body: err.Error()} // network errors are retryable
	}
	defer httpResp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(httpResp.Body, 10<<20))
	if err != nil {
		return Response{}, err
	}
	if httpResp.StatusCode != http.StatusOK {
		return Response{}, &APIError{Status: httpResp.StatusCode, Body: string(raw)}
	}
	return parseWireResponse(raw)
}

func parseWireResponse(raw []byte) (Response, error) {
	var wr wireResponse
	if err := json.Unmarshal(raw, &wr); err != nil {
		return Response{}, fmt.Errorf("decode gemini response: %w", err)
	}
	if wr.PromptFeedback != nil && wr.PromptFeedback.BlockReason != "" {
		return Response{}, fmt.Errorf("gemini blocked the prompt: %s", wr.PromptFeedback.BlockReason)
	}
	if len(wr.Candidates) == 0 {
		return Response{}, errors.New("gemini returned no candidates")
	}
	c := wr.Candidates[0]
	var sb strings.Builder
	for _, p := range c.Content.Parts {
		sb.WriteString(p.Text)
	}
	out := Response{Text: sb.String()}
	if out.Text == "" {
		return Response{}, fmt.Errorf("gemini returned an empty answer (finish reason %q)", c.FinishReason)
	}
	if c.Grounding != nil {
		out.SearchQueries = c.Grounding.WebSearchQueries
		for _, ch := range c.Grounding.GroundingChunks {
			if ch.Web != nil {
				out.Sources = append(out.Sources, Source{Title: ch.Web.Title, URI: ch.Web.URI})
			}
		}
	}
	return out, nil
}

// DecodeJSON parses a model answer into v, tolerating markdown code fences
// and stray text around the JSON object.
func DecodeJSON(text string, v any) error {
	s := strings.TrimSpace(text)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	if err := json.Unmarshal([]byte(s), v); err == nil {
		return nil
	}
	start, end := strings.Index(s, "{"), strings.LastIndex(s, "}")
	if start == -1 || end <= start {
		return errors.New("no JSON object in model answer")
	}
	if err := json.Unmarshal([]byte(s[start:end+1]), v); err != nil {
		return fmt.Errorf("model answer is not valid JSON: %w", err)
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
