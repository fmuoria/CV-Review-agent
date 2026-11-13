package intake

import "testing"

func TestGroupUploadsPairsByConvention(t *testing.T) {
	groups := GroupUploads([]File{
		{Name: "JaneSmith_CV.pdf"}, {Name: "JaneSmith_CoverLetter.pdf"},
		{Name: "john-doe-resume.docx"}, {Name: "scan001.pdf"},
	})
	if len(groups) != 3 {
		t.Fatalf("got %d groups: %+v", len(groups), groups)
	}
	for _, g := range groups {
		if g.Key == "name:janesmith" {
			if len(g.Files) != 2 || g.Files[0].Kind != "cv" || g.Files[1].Kind != "cover_letter" {
				t.Fatalf("jane not paired: %+v", g.Files)
			}
		}
	}
}

func TestAssignKinds(t *testing.T) {
	got := AssignKinds([]File{{Name: "Motivation letter.pdf"}, {Name: "Document1.pdf"}, {Name: "certificates.pdf"}})
	want := []string{"cover_letter", "cv", "other"}
	for i := range want {
		if got[i].Kind != want[i] {
			t.Fatalf("file %d kind = %s, want %s", i, got[i].Kind, want[i])
		}
	}
}

func TestCleanName(t *testing.T) {
	if got := cleanName(`..\..\evil<script>.pdf`); got != "evil_script_.pdf" {
		t.Fatalf("cleanName = %q", got)
	}
}
