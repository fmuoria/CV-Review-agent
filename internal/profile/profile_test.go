package profile

import (
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)

func boolp(b bool) *bool { return &b }

func codes(fs []Flag) map[string]int {
	m := map[string]int{}
	for _, f := range fs {
		m[f.Code]++
	}
	return m
}

// The old prompt treated "Present" as Nov 2025. A role from Feb 2021 to present
// is 70 months as of Nov 2026.
func TestPresentUsesToday(t *testing.T) {
	p := Profile{Email: "a@b.co", Employment: []Role{{Title: "Officer", Start: "2021-02", End: "present"}}}
	p.Analyse(now)
	if got := p.Employment[0].DurationMonths; got != 70 {
		t.Fatalf("duration = %d, want 70", got)
	}
	if p.Employment[0].EndLabel != "Present" {
		t.Fatalf("end label = %q", p.Employment[0].EndLabel)
	}
}

// A job that started in March 2026 used to be flagged as a "future date".
func TestRecentStartIsNotFuture(t *testing.T) {
	p := Profile{Email: "a@b.co", Employment: []Role{{Title: "Officer", Start: "2026-03", End: "present"}}}
	if c := codes(p.Analyse(now)); c["future_date"] > 0 {
		t.Fatal("2026-03 flagged as future")
	}
	p2 := Profile{Email: "a@b.co", Employment: []Role{{Title: "Officer", Start: "2027-01", End: "present"}}}
	if c := codes(p2.Analyse(now)); c["future_date"] != 1 {
		t.Fatal("2027-01 should be flagged as future")
	}
}

func TestTotalCountsOverlapsOnce(t *testing.T) {
	p := Profile{Email: "a@b.co", Employment: []Role{
		{Title: "A", Start: "2020-01", End: "2021-12"}, // 24 months
		{Title: "B", Start: "2021-07", End: "2022-12"}, // overlaps 6 months, adds 12
	}}
	p.Analyse(now)
	if p.TotalMonths != 36 {
		t.Fatalf("total = %d, want 36", p.TotalMonths)
	}
}

func TestDateOrderAndConcurrentRoles(t *testing.T) {
	p := Profile{Email: "a@b.co", Employment: []Role{
		{Title: "Backwards", Start: "2022-05", End: "2021-01"},
		{Title: "A", Start: "2023-01", End: "2024-12", FullTime: boolp(true)},
		{Title: "B", Start: "2023-01", End: "2024-12", FullTime: boolp(true)},
		{Title: "C", Start: "2023-01", End: "2024-12", FullTime: boolp(true)},
		{Title: "Volunteer", Start: "2023-01", End: "2024-12", FullTime: boolp(false)},
	}}
	c := codes(p.Analyse(now))
	if c["date_order"] != 1 {
		t.Error("expected date_order flag")
	}
	if c["concurrent_roles"] != 1 {
		t.Error("expected concurrent_roles flag for 3 full-time roles")
	}
}

func TestGapFlag(t *testing.T) {
	p := Profile{Email: "a@b.co", Employment: []Role{
		{Title: "A", Start: "2015-01", End: "2016-12"},
		{Title: "B", Start: "2019-01", End: "present"},
	}}
	if c := codes(p.Analyse(now)); c["gap"] != 1 {
		t.Fatalf("expected one gap flag, got %v", c)
	}
}

func TestYearOnlyDates(t *testing.T) {
	p := Profile{Email: "a@b.co", Employment: []Role{{Title: "A", Start: "2020", End: "2024"}}}
	p.Analyse(now)
	if p.Employment[0].DurationMonths != 60 {
		t.Fatalf("2020-2024 should be 60 months, got %d", p.Employment[0].DurationMonths)
	}
}

func TestTimelineSummary(t *testing.T) {
	p := Profile{Email: "a@b.co", Employment: []Role{{Title: "Officer", Employer: "Sinapis", Start: "2021-02", End: "present"}}}
	p.Analyse(now)
	s := p.TimelineSummary(now)
	for _, want := range []string{"12 November 2026", "Officer — Sinapis", "Feb 2021 to Present", "5 yrs 10 mos"} {
		if !strings.Contains(s, want) {
			t.Errorf("summary missing %q:\n%s", want, s)
		}
	}
}

func TestDemoHandlerFindsBasics(t *testing.T) {
	cv := "Jane Wanjiru\njane@example.org | +254 700 123 456\nlinkedin.com/in/janew\nProgramme Officer, Acme  Jan 2020 - Present\n"
	out, err := DemoHandler(llmReq(cv))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Jane Wanjiru", "jane@example.org", "linkedin.com/in/janew", "2020-01", "present"} {
		if !strings.Contains(out, want) {
			t.Errorf("demo profile missing %q: %s", want, out)
		}
	}
}

func TestFormatMonths(t *testing.T) {
	for m, want := range map[int]string{1: "1 mo", 5: "5 mos", 12: "1 yr", 13: "1 yr 1 mo", 20: "1 yr 8 mos", 52: "4 yrs 4 mos", 0: "0 mos"} {
		if got := FormatMonths(m); got != want {
			t.Errorf("FormatMonths(%d) = %q, want %q", m, got, want)
		}
	}
}
