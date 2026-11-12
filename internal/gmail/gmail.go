// Package gmail reads job applications from a connected Gmail inbox
// (e.g. careers@sinapis.org) using the Gmail REST API, read-only.
//
// Fixes compared with the desktop agent:
//   - Applicants are identified by their email address, not their display
//     name, so two "John Kamau"s no longer overwrite each other.
//   - Attachments are found at any depth (forwarded emails, mixed/alternative
//     parts), not just the top level.
//   - The email body is kept and used as the cover letter when no cover
//     letter is attached (common for "send your CV to …" adverts).
package gmail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"
)

// ReadonlyScope is the only Gmail permission the app asks for.
const ReadonlyScope = "https://www.googleapis.com/auth/gmail.readonly"

const apiBase = "https://gmail.googleapis.com/gmail/v1/users/me"

// Client calls the Gmail API with an access token.
type Client struct {
	Token   string
	HTTP    *http.Client
	BaseURL string // tests
}

// Attachment is one file in an email.
type Attachment struct {
	Filename string
	MIMEType string
	Data     []byte
}

// Message is an application email.
type Message struct {
	ID          string
	FromName    string
	FromEmail   string
	Subject     string
	Date        time.Time
	Body        string
	Attachments []Attachment
}

type part struct {
	PartID   string `json:"partId"`
	MimeType string `json:"mimeType"`
	Filename string `json:"filename"`
	Headers  []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"headers"`
	Body struct {
		AttachmentID string `json:"attachmentId"`
		Size         int    `json:"size"`
		Data         string `json:"data"`
	} `json:"body"`
	Parts []part `json:"parts"`
}

type rawMessage struct {
	ID           string `json:"id"`
	InternalDate string `json:"internalDate"`
	Payload      part   `json:"payload"`
}

func (c *Client) base() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return apiBase
}

func (c *Client) get(ctx context.Context, path string, q url.Values, v any) error {
	u := c.base() + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: time.Minute}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 60<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gmail API %s: %d %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.Unmarshal(body, v)
}

// BuildQuery makes a Gmail search query for application emails.
func BuildQuery(subject string, since time.Time) string {
	q := []string{"has:attachment"}
	if s := strings.TrimSpace(subject); s != "" {
		q = append(q, fmt.Sprintf("subject:(%q)", s))
	}
	if !since.IsZero() {
		q = append(q, "after:"+since.Format("2006/01/02"))
	}
	return strings.Join(q, " ")
}

// ListIDs returns up to max message ids matching the query, newest first.
func (c *Client) ListIDs(ctx context.Context, query string, max int) ([]string, error) {
	var ids []string
	pageToken := ""
	for len(ids) < max {
		q := url.Values{"q": {query}, "maxResults": {"100"}}
		if pageToken != "" {
			q.Set("pageToken", pageToken)
		}
		var res struct {
			Messages []struct {
				ID string `json:"id"`
			} `json:"messages"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := c.get(ctx, "/messages", q, &res); err != nil {
			return nil, err
		}
		for _, m := range res.Messages {
			ids = append(ids, m.ID)
			if len(ids) >= max {
				break
			}
		}
		if res.NextPageToken == "" {
			break
		}
		pageToken = res.NextPageToken
	}
	return ids, nil
}

// Get fetches a message with its attachments.
func (c *Client) Get(ctx context.Context, id string) (Message, error) {
	var raw rawMessage
	if err := c.get(ctx, "/messages/"+url.PathEscape(id), url.Values{"format": {"full"}}, &raw); err != nil {
		return Message{}, err
	}
	msg := Message{ID: raw.ID}
	for _, h := range raw.Payload.Headers {
		switch strings.ToLower(h.Name) {
		case "from":
			msg.FromName, msg.FromEmail = parseFrom(h.Value)
		case "subject":
			msg.Subject = h.Value
		case "date":
			if t, err := mail.ParseDate(h.Value); err == nil {
				msg.Date = t
			}
		}
	}

	var plain, html string
	var files []part
	walk(raw.Payload, func(p part) {
		switch {
		case p.Filename != "":
			files = append(files, p)
		case p.MimeType == "text/plain" && plain == "":
			plain = decode(p.Body.Data)
		case p.MimeType == "text/html" && html == "":
			html = decode(p.Body.Data)
		}
	})
	msg.Body = strings.TrimSpace(plain)
	if msg.Body == "" && html != "" {
		msg.Body = stripHTML(html)
	}

	for _, f := range files {
		data := []byte(decode(f.Body.Data))
		if f.Body.AttachmentID != "" {
			var att struct {
				Data string `json:"data"`
			}
			if err := c.get(ctx, "/messages/"+url.PathEscape(id)+"/attachments/"+url.PathEscape(f.Body.AttachmentID), nil, &att); err != nil {
				return msg, err
			}
			data = []byte(decode(att.Data))
		}
		if len(data) == 0 {
			continue
		}
		msg.Attachments = append(msg.Attachments, Attachment{Filename: f.Filename, MIMEType: f.MimeType, Data: data})
	}
	return msg, nil
}

func walk(p part, fn func(part)) {
	fn(p)
	for _, child := range p.Parts {
		walk(child, fn)
	}
}

func decode(s string) string {
	if s == "" {
		return ""
	}
	b, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		b, err = base64.RawURLEncoding.DecodeString(s)
		if err != nil {
			return ""
		}
	}
	return string(b)
}

func parseFrom(v string) (string, string) {
	addr, err := mail.ParseAddress(v)
	if err != nil {
		if i := strings.Index(v, "<"); i >= 0 {
			return strings.TrimSpace(strings.Trim(v[:i], `" `)), strings.ToLower(strings.Trim(v[i:], "<> "))
		}
		return "", strings.ToLower(strings.TrimSpace(v))
	}
	return addr.Name, strings.ToLower(addr.Address)
}

func stripHTML(s string) string {
	var sb strings.Builder
	in := false
	for _, r := range s {
		switch {
		case r == '<':
			in = true
		case r == '>':
			in = false
			sb.WriteByte(' ')
		case !in:
			sb.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(sb.String()), " ")
}
