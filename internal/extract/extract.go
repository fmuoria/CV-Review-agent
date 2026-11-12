// Package extract turns uploaded CVs and cover letters into plain text.
//
// Order of attempts for a PDF:
//  1. pdftotext (poppler) — fast and free; installed in the Docker image.
//  2. If that is missing or yields almost no text (a scanned/photographed CV),
//     Gemini reads the PDF directly ("ai_ocr").
//
// The desktop agent silently skipped such files; here they are transcribed,
// or the application is marked "needs review" so HR can look at it by hand.
package extract

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/fmuoria/CV-Review-agent/internal/llm"
)

// Methods recorded on each document.
const (
	MethodPlain     = "plain"
	MethodPDFToText = "pdftotext"
	MethodDOCX      = "docx"
	MethodAntiword  = "antiword"
	MethodAIOCR     = "ai_ocr"
)

// MinUsefulChars is the least amount of text we accept as a real extraction.
const MinUsefulChars = 200

// ErrUnreadable means no method produced usable text.
var ErrUnreadable = errors.New("could not read any text from this file")

// Kind is the detected file type.
type Kind string

// Supported kinds.
const (
	KindPDF     Kind = "pdf"
	KindDOCX    Kind = "docx"
	KindDOC     Kind = "doc"
	KindText    Kind = "txt"
	KindImage   Kind = "image"
	KindUnknown Kind = "unknown"
)

// Detect sniffs the file type from its bytes, using the extension only as a tie-breaker.
func Detect(filename string, data []byte) (Kind, string) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch {
	case bytes.HasPrefix(data, []byte("%PDF-")):
		return KindPDF, "application/pdf"
	case bytes.HasPrefix(data, []byte("PK\x03\x04")) && (ext == ".docx" || zipHasDocument(data)):
		return KindDOCX, "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case bytes.HasPrefix(data, []byte{0xD0, 0xCF, 0x11, 0xE0}): // OLE2 = legacy .doc
		return KindDOC, "application/msword"
	}
	ct := http.DetectContentType(data)
	switch {
	case ct == "image/jpeg" || ct == "image/png" || ct == "image/webp":
		return KindImage, ct
	case strings.HasPrefix(ct, "text/plain") || ext == ".txt":
		if utf8.Valid(data) || ext == ".txt" {
			return KindText, "text/plain"
		}
	}
	return KindUnknown, ct
}

// Allowed reports whether the kind is accepted for upload.
func Allowed(k Kind) bool {
	switch k {
	case KindPDF, KindDOCX, KindDOC, KindText, KindImage:
		return true
	}
	return false
}

// Extractor converts files to text. AI may be nil (no OCR fallback).
type Extractor struct {
	AI llm.Client
}

// Text extracts text from a document and reports the method used.
func (e *Extractor) Text(ctx context.Context, filename string, data []byte) (string, string, error) {
	kind, mime := Detect(filename, data)
	switch kind {
	case KindText:
		return clean(strings.ToValidUTF8(string(data), "�")), MethodPlain, nil
	case KindDOCX:
		text, err := docxText(data)
		if err != nil {
			return "", "", fmt.Errorf("read DOCX: %w", err)
		}
		if usable(text) {
			return text, MethodDOCX, nil
		}
		return "", "", ErrUnreadable
	case KindDOC:
		if text, err := runTool(ctx, "antiword", data, ".doc"); err == nil && usable(text) {
			return text, MethodAntiword, nil
		}
		return "", "", fmt.Errorf("%w: legacy .doc files need antiword; ask the candidate for PDF or DOCX", ErrUnreadable)
	case KindPDF:
		if text, err := runTool(ctx, "pdftotext", data, ".pdf", "-layout"); err == nil && usable(text) {
			return text, MethodPDFToText, nil
		}
		return e.aiTranscribe(ctx, data, mime)
	case KindImage:
		return e.aiTranscribe(ctx, data, mime)
	}
	return "", "", fmt.Errorf("unsupported file type (%s)", mime)
}

func (e *Extractor) aiTranscribe(ctx context.Context, data []byte, mime string) (string, string, error) {
	if e.AI == nil {
		return "", "", ErrUnreadable
	}
	if len(data) > 18<<20 {
		return "", "", fmt.Errorf("%w: file too large for AI transcription", ErrUnreadable)
	}
	resp, err := e.AI.Generate(ctx, llm.Request{
		Task:   "transcribe",
		System: "You transcribe documents. Output only the document's text, preserving headings, dates and bullet points. Do not summarise, add commentary or invent text.",
		Parts: []llm.Part{
			{Text: "Transcribe this CV or cover letter to plain text."},
			{MIMEType: mime, Data: data},
		},
		Hint: data,
	})
	if err != nil {
		return "", "", fmt.Errorf("AI transcription failed: %w", err)
	}
	if !usable(resp.Text) {
		return "", "", ErrUnreadable
	}
	return clean(resp.Text), MethodAIOCR, nil
}

func usable(text string) bool {
	return len(strings.Join(strings.Fields(text), " ")) >= MinUsefulChars
}

// runTool writes data to a temp file and runs a command-line converter on it.
func runTool(ctx context.Context, tool string, data []byte, ext string, args ...string) (string, error) {
	path, err := exec.LookPath(tool)
	if err != nil {
		return "", err
	}
	f, err := os.CreateTemp("", "cv-*"+ext)
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return "", err
	}
	f.Close()
	all := append(append([]string{}, args...), f.Name())
	if tool == "pdftotext" {
		all = append(all, "-") // write to stdout
	}
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, path, all...)
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return clean(strings.ToValidUTF8(out.String(), "�")), nil
}

func zipHasDocument(data []byte) bool {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return false
	}
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			return true
		}
	}
	return false
}

// docxText reads word/document.xml and keeps paragraph, tab and line breaks.
func docxText(data []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", err
	}
	var doc *zip.File
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			doc = f
			break
		}
	}
	if doc == nil {
		return "", errors.New("word/document.xml not found")
	}
	rc, err := doc.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()

	dec := xml.NewDecoder(io.LimitReader(rc, 50<<20))
	var sb strings.Builder
	inText := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "t":
				inText = true
			case "tab":
				sb.WriteByte('\t')
			case "br", "cr":
				sb.WriteByte('\n')
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				inText = false
			case "p":
				sb.WriteByte('\n')
			}
		case xml.CharData:
			if inText {
				sb.Write(t)
			}
		}
	}
	return clean(sb.String()), nil
}

// clean normalises line endings and collapses runs of blank lines.
func clean(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\x00", "")
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	blank := 0
	for _, l := range lines {
		l = strings.TrimRight(l, " \t")
		if l == "" {
			blank++
			if blank > 1 {
				continue
			}
		} else {
			blank = 0
		}
		out = append(out, l)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}
