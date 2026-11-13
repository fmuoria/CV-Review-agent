// Package intake adds applications from any channel — the careers form,
// HR bulk upload, or the Gmail import — through one validated path.
package intake

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/fmuoria/CV-Review-agent/internal/extract"
	"github.com/fmuoria/CV-Review-agent/internal/storage"
	"github.com/fmuoria/CV-Review-agent/internal/store"
)

// File is an uploaded document.
type File struct {
	Name string
	Data []byte
	Kind string // cv, cover_letter, linkedin_pdf, other
}

// Service saves applications and their files.
type Service struct {
	Store    *store.Store
	Files    storage.Store
	MaxBytes int64
	OnSaved  func() // e.g. wake the screening workers
}

// ValidationError is shown to the person submitting.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

// Check validates a file before anything is saved.
func (s *Service) Check(f File) error {
	if len(f.Data) == 0 {
		return &ValidationError{fmt.Sprintf("%s is empty.", f.Name)}
	}
	if s.MaxBytes > 0 && int64(len(f.Data)) > s.MaxBytes {
		return &ValidationError{fmt.Sprintf("%s is larger than %d MB.", f.Name, s.MaxBytes>>20)}
	}
	if k, _ := extract.Detect(f.Name, f.Data); !extract.Allowed(k) {
		return &ValidationError{fmt.Sprintf("%s is not a PDF, Word document, text file or image.", f.Name)}
	}
	return nil
}

// Submit creates the application, stores its files and queues screening.
func (s *Service) Submit(ctx context.Context, app store.NewApplication, files []File) (int64, error) {
	hasCV := false
	for _, f := range files {
		if err := s.Check(f); err != nil {
			return 0, err
		}
		if f.Kind == "cv" {
			hasCV = true
		}
	}
	if !hasCV {
		return 0, &ValidationError{"A CV is required."}
	}
	id, err := s.Store.CreateApplication(ctx, app)
	if err != nil {
		return 0, err
	}
	for _, f := range files {
		_, ct := extract.Detect(f.Name, f.Data)
		key := fmt.Sprintf("applications/%d/%s-%s%s", id, f.Kind, randomHex(6), safeExt(f.Name))
		if err := s.Files.Put(ctx, key, f.Data, ct); err != nil {
			return id, fmt.Errorf("store %s: %w", f.Name, err)
		}
		doc := store.Document{ApplicationID: id, Kind: f.Kind, Filename: cleanName(f.Name), ContentType: ct,
			SizeBytes: int64(len(f.Data)), StorageKey: key}
		if err := s.Store.AddDocument(ctx, &doc); err != nil {
			return id, err
		}
	}
	if s.OnSaved != nil {
		s.OnSaved()
	}
	return id, nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func safeExt(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".pdf", ".docx", ".doc", ".txt", ".jpg", ".jpeg", ".png", ".webp":
		return ext
	}
	return ""
}

var reUnsafe = regexp.MustCompile(`[^\w.\- ()]+`)

func cleanName(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = reUnsafe.ReplaceAllString(name, "_")
	if len(name) > 120 {
		name = name[len(name)-120:]
	}
	if name == "" || name == "." {
		name = "document"
	}
	return name
}

// ClassifyAttachment guesses whether a file is a CV or cover letter from its name.
func ClassifyAttachment(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "cover") || strings.Contains(n, "letter") || strings.Contains(n, "motivation"):
		return "cover_letter"
	case strings.Contains(n, "linkedin"):
		return "linkedin_pdf"
	case strings.Contains(n, "cv") || strings.Contains(n, "resume") || strings.Contains(n, "résumé") || strings.Contains(n, "curriculum"):
		return "cv"
	}
	return ""
}

// AssignKinds labels a set of attachments from one applicant. Named files are
// trusted; otherwise the first unlabelled file becomes the CV.
func AssignKinds(files []File) []File {
	out := append([]File(nil), files...)
	hasCV := false
	for i := range out {
		if out[i].Kind == "" {
			out[i].Kind = ClassifyAttachment(out[i].Name)
		}
		if out[i].Kind == "cv" {
			hasCV = true
		}
	}
	for i := range out {
		if out[i].Kind == "" {
			if !hasCV {
				out[i].Kind, hasCV = "cv", true
			} else {
				out[i].Kind = "other"
			}
		}
	}
	return out
}

// UploadGroup is one applicant's files in a bulk upload.
type UploadGroup struct {
	Key   string
	Files []File
}

var reConvention = regexp.MustCompile(`(?i)^(.+?)[_\- ]+(cv|resume|coverletter|cover[_\- ]letter|letter)$`)

// GroupUploads pairs bulk-uploaded files into applicants. Files named with the
// desktop agent's convention ("JaneDoe_CV.pdf" + "JaneDoe_CoverLetter.pdf")
// are paired; any other file is treated as one applicant's CV.
func GroupUploads(files []File) []UploadGroup {
	groups := map[string]*UploadGroup{}
	var order []string
	for _, f := range files {
		base := strings.TrimSuffix(filepath.Base(f.Name), filepath.Ext(f.Name))
		key, kind := "file:"+f.Name, "cv"
		if m := reConvention.FindStringSubmatch(base); m != nil {
			key = "name:" + strings.ToLower(m[1])
			if t := strings.ToLower(m[2]); t != "cv" && t != "resume" {
				kind = "cover_letter"
			}
		}
		f.Kind = kind
		g, ok := groups[key]
		if !ok {
			g = &UploadGroup{Key: key}
			groups[key] = g
			order = append(order, key)
		}
		g.Files = append(g.Files, f)
	}
	sort.Strings(order)
	out := make([]UploadGroup, 0, len(order))
	for _, k := range order {
		out = append(out, *groups[k])
	}
	return out
}
