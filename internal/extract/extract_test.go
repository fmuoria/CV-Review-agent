package extract

import (
	"archive/zip"
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/fmuoria/CV-Review-agent/internal/llm"
)

func makeDocx(t *testing.T, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("word/document.xml")
	w.Write([]byte(`<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` + body + `</w:body></w:document>`))
	zw.Close()
	return buf.Bytes()
}

func TestDocxText(t *testing.T) {
	long := strings.Repeat("Delivered business training to entrepreneurs. ", 10)
	data := makeDocx(t, `<w:p><w:r><w:t>Jane Wanjiru</w:t></w:r></w:p><w:p><w:r><w:t>Programme Officer</w:t><w:tab/><w:t>2021 - Present</w:t></w:r></w:p><w:p><w:r><w:t>`+long+`</w:t></w:r></w:p>`)
	if k, _ := Detect("cv.docx", data); k != KindDOCX {
		t.Fatalf("detected %v", k)
	}
	text, method, err := (&Extractor{}).Text(context.Background(), "cv.docx", data)
	if err != nil {
		t.Fatal(err)
	}
	if method != MethodDOCX || !strings.Contains(text, "Jane Wanjiru\nProgramme Officer\t2021 - Present") {
		t.Fatalf("unexpected text (%s): %q", method, text[:80])
	}
}

func TestDetect(t *testing.T) {
	cases := map[string]Kind{
		"%PDF-1.7 rest":   KindPDF,
		"Plain CV text\n": KindText,
	}
	for in, want := range cases {
		if got, _ := Detect("file", []byte(in)); got != want {
			t.Errorf("Detect(%q) = %v, want %v", in, got, want)
		}
	}
	if k, _ := Detect("x.exe", []byte{0x4d, 0x5a, 0x90, 0x00, 0x03}); Allowed(k) {
		t.Error("binary accepted")
	}
}

type ocrAI struct{ called bool }

func (o *ocrAI) Generate(_ context.Context, r llm.Request) (llm.Response, error) {
	o.called = true
	return llm.Response{Text: strings.Repeat("Transcribed CV text from a scanned page. ", 10)}, nil
}
func (o *ocrAI) Model() string { return "fake" }

// Fix 6: a PDF with no extractable text is sent to the AI instead of being skipped.
func TestScannedPDFFallsBackToAI(t *testing.T) {
	ai := &ocrAI{}
	fake := []byte("%PDF-1.4\n% no text layer\n")
	text, method, err := (&Extractor{AI: ai}).Text(context.Background(), "scan.pdf", fake)
	if err != nil {
		t.Fatal(err)
	}
	if !ai.called || method != MethodAIOCR || !strings.Contains(text, "Transcribed") {
		t.Fatalf("fallback not used: method=%s", method)
	}
	if _, _, err := (&Extractor{}).Text(context.Background(), "scan.pdf", fake); err == nil {
		t.Fatal("without AI an unreadable PDF should error")
	}
}
