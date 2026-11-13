// Package profile pulls structured facts out of a CV (contacts, employment
// timeline, education) and runs consistency checks on the timeline.
//
// The AI only *reads* dates off the CV. All arithmetic — durations, overlaps,
// gaps, future dates — is done here in Go, where it is exact and testable.
// The old prompt asked the model to do date maths (and hardcoded the current
// date as 22 Nov 2025); this replaces that.
package profile

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fmuoria/CV-Review-agent/internal/llm"
)

// Profile is the structured view of a CV.
type Profile struct {
	Name           string      `json:"name"`
	Email          string      `json:"email"`
	Phone          string      `json:"phone"`
	Location       string      `json:"location"`
	LinkedInURL    string      `json:"linkedin_url"`
	Links          []string    `json:"links"`
	Employment     []Role      `json:"employment"`
	Education      []Education `json:"education"`
	Certifications []string    `json:"certifications"`
	Skills         []string    `json:"skills"`

	// Computed by Analyse.
	TotalMonths int `json:"total_months"`
}

// Role is one job on the CV.
type Role struct {
	Title    string `json:"title"`
	Employer string `json:"employer"`
	Location string `json:"location"`
	Start    string `json:"start"` // "YYYY-MM", "YYYY" or ""
	End      string `json:"end"`   // "YYYY-MM", "YYYY", "present" or ""
	FullTime *bool  `json:"full_time"`

	// Computed by Analyse.
	StartLabel     string `json:"start_label,omitempty"`
	EndLabel       string `json:"end_label,omitempty"`
	DurationMonths int    `json:"duration_months"`
	DatesValid     bool   `json:"dates_valid"`
}

// Education is one qualification.
type Education struct {
	Qualification string `json:"qualification"`
	Field         string `json:"field"`
	Institution   string `json:"institution"`
	Year          string `json:"year"`
}

// Flag is a consistency observation for a human to look at. Flags never
// change the score.
type Flag struct {
	Severity string `json:"severity"` // "warning" or "info"
	Code     string `json:"code"`
	Message  string `json:"message"`
}

const systemPrompt = `You extract facts from CVs into JSON for a recruitment system.
Rules:
- Copy facts exactly as they appear. Never guess, infer or invent. Use "" or [] when something is not in the CV.
- Dates: convert to "YYYY-MM" when month and year are given, "YYYY" when only the year is given,
  "present" for Present/Current/Ongoing/To date. Interpret "08/2025" as 2025-08, "Aug '21" as 2021-08,
  "Q1 2024" as 2024-01. For DD/MM/YYYY vs MM/DD/YYYY: if the first number is >12 it is the day.
- full_time: true if the role is clearly a full-time job, false for part-time, internships, volunteering,
  consultancies or board roles, null if unclear.
- Read the whole document: sections can appear in any order, in columns, headers or footers.
- Do not include personal characteristics (age, religion, marital status, ethnicity, photos, health) in any field.`

const schema = `Return JSON with exactly these keys:
{
  "name": "", "email": "", "phone": "", "location": "", "linkedin_url": "",
  "links": [""],
  "employment": [{"title": "", "employer": "", "location": "", "start": "", "end": "", "full_time": true}],
  "education": [{"qualification": "", "field": "", "institution": "", "year": ""}],
  "certifications": [""],
  "skills": [""]
}
List employment most recent first.`

// Extract asks the AI for a structured profile of the CV text.
func Extract(ctx context.Context, ai llm.Client, cvText string) (Profile, error) {
	resp, err := ai.Generate(ctx, llm.Request{
		Task:   "profile",
		System: systemPrompt,
		JSON:   true,
		Parts:  []llm.Part{{Text: schema + "\n\nCV:\n" + clip(cvText, 30000)}},
		Hint:   cvText,
	})
	if err != nil {
		return Profile{}, fmt.Errorf("profile extraction: %w", err)
	}
	var p Profile
	if err := llm.DecodeJSON(resp.Text, &p); err != nil {
		return Profile{}, fmt.Errorf("profile extraction: %w", err)
	}
	p.normalise()
	return p, nil
}

func (p *Profile) normalise() {
	p.Email = strings.ToLower(strings.TrimSpace(p.Email))
	p.Name = strings.Join(strings.Fields(p.Name), " ")
	p.LinkedInURL = strings.TrimSpace(p.LinkedInURL)
	if p.LinkedInURL == "" {
		for _, l := range p.Links {
			if strings.Contains(strings.ToLower(l), "linkedin.com/") {
				p.LinkedInURL = l
				break
			}
		}
	}
}

// month is a count of months since year 0, so arithmetic is simple.
type month int

func (m month) label() string {
	y, mo := int(m)/12, int(m)%12+1
	return time.Month(mo).String()[:3] + " " + strconv.Itoa(y)
}

func monthOf(t time.Time) month { return month(t.Year()*12 + int(t.Month()) - 1) }

// parseDate turns "YYYY-MM", "YYYY" or "present" into a month. For a bare
// year, isEnd picks December (or the current month for this year) instead of January.
func parseDate(s string, isEnd bool, now month) (month, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return 0, false
	}
	if s == "present" || s == "current" || s == "now" {
		return now, true
	}
	if y, err := strconv.Atoi(s); err == nil && len(s) == 4 {
		if !isEnd {
			return month(y * 12), true
		}
		end := month(y*12 + 11)
		if end > now && y == int(now)/12 {
			end = now
		}
		return end, true
	}
	if len(s) == 7 && s[4] == '-' {
		y, err1 := strconv.Atoi(s[:4])
		m, err2 := strconv.Atoi(s[5:])
		if err1 == nil && err2 == nil && m >= 1 && m <= 12 {
			return month(y*12 + m - 1), true
		}
	}
	return 0, false
}

type interval struct {
	start, end month
	fullTime   bool
	idx        int
}

// Analyse computes durations and total experience, and returns flags.
// now is passed in (rather than read from the clock) so tests are exact.
func (p *Profile) Analyse(now time.Time) []Flag {
	cur := monthOf(now)
	var flags []Flag
	var spans []interval
	undated := 0

	for i := range p.Employment {
		r := &p.Employment[i]
		start, okS := parseDate(r.Start, false, cur)
		end, okE := parseDate(r.End, true, cur)
		if !okS {
			undated++
			continue
		}
		if !okE {
			// A start date with no end usually means a single-month or unclear role; treat as one month.
			end = start
		}
		r.StartLabel, r.EndLabel = start.label(), end.label()
		if strings.EqualFold(strings.TrimSpace(r.End), "present") {
			r.EndLabel = "Present"
		}
		who := roleName(*r)
		switch {
		case start > cur:
			flags = append(flags, Flag{"warning", "future_date",
				fmt.Sprintf("%s starts in the future (%s).", who, start.label())})
			continue
		case end < start:
			flags = append(flags, Flag{"warning", "date_order",
				fmt.Sprintf("%s ends (%s) before it starts (%s).", who, end.label(), start.label())})
			continue
		case int(end-start) > 600:
			flags = append(flags, Flag{"warning", "implausible_duration",
				fmt.Sprintf("%s lasts over 50 years — likely a date error.", who)})
			continue
		}
		r.DatesValid = true
		r.DurationMonths = int(end-start) + 1
		ft := r.FullTime == nil || *r.FullTime // unknown counts as full-time for overlap checks
		spans = append(spans, interval{start, end, ft, i})
	}

	if undated > 0 {
		flags = append(flags, Flag{"info", "missing_dates",
			fmt.Sprintf("%d role(s) have no dates, so their length could not be counted.", undated)})
	}

	p.TotalMonths = unionMonths(spans)
	flags = append(flags, overlapFlags(p.Employment, spans)...)
	flags = append(flags, gapFlags(spans, cur)...)

	if p.Email == "" {
		flags = append(flags, Flag{"info", "no_email", "No email address found on the CV."})
	}
	return flags
}

func roleName(r Role) string {
	switch {
	case r.Title != "" && r.Employer != "":
		return fmt.Sprintf("%q at %s", r.Title, r.Employer)
	case r.Title != "":
		return fmt.Sprintf("%q", r.Title)
	case r.Employer != "":
		return "The role at " + r.Employer
	}
	return "A role"
}

// unionMonths counts distinct months worked, so overlapping jobs are not double-counted.
func unionMonths(spans []interval) int {
	if len(spans) == 0 {
		return 0
	}
	s := append([]interval(nil), spans...)
	sort.Slice(s, func(i, j int) bool { return s[i].start < s[j].start })
	total := 0
	curStart, curEnd := s[0].start, s[0].end
	for _, iv := range s[1:] {
		if iv.start <= curEnd+1 {
			if iv.end > curEnd {
				curEnd = iv.end
			}
			continue
		}
		total += int(curEnd-curStart) + 1
		curStart, curEnd = iv.start, iv.end
	}
	return total + int(curEnd-curStart) + 1
}

func overlapFlags(roles []Role, spans []interval) []Flag {
	var ft []interval
	for _, s := range spans {
		if s.fullTime {
			ft = append(ft, s)
		}
	}
	var flags []Flag
	// 3+ full-time roles at the same time (the check from the desktop agent).
	maxConcurrent, at := 0, month(0)
	for _, a := range ft {
		n := 0
		for _, b := range ft {
			if b.start <= a.start && a.start <= b.end {
				n++
			}
		}
		if n > maxConcurrent {
			maxConcurrent, at = n, a.start
		}
	}
	if maxConcurrent >= 3 {
		flags = append(flags, Flag{"warning", "concurrent_roles",
			fmt.Sprintf("CV shows %d full-time roles at the same time (around %s), which is unusual.", maxConcurrent, at.label())})
	}
	// Pairs of full-time roles overlapping by more than 6 months.
	for i := 0; i < len(ft); i++ {
		for j := i + 1; j < len(ft); j++ {
			lo, hi := maxM(ft[i].start, ft[j].start), minM(ft[i].end, ft[j].end)
			if int(hi-lo)+1 > 6 {
				flags = append(flags, Flag{"info", "overlap",
					fmt.Sprintf("%s and %s overlap for %d months. Worth confirming both were full-time.",
						roleName(roles[ft[i].idx]), roleName(roles[ft[j].idx]), int(hi-lo)+1)})
			}
		}
	}
	return flags
}

func gapFlags(spans []interval, cur month) []Flag {
	if len(spans) == 0 {
		return nil
	}
	s := append([]interval(nil), spans...)
	sort.Slice(s, func(i, j int) bool { return s[i].start < s[j].start })
	var flags []Flag
	reach := s[0].end
	for _, iv := range s[1:] {
		if gap := int(iv.start-reach) - 1; gap > 12 {
			flags = append(flags, Flag{"info", "gap",
				fmt.Sprintf("Gap of %d months between %s and %s. Not necessarily a concern — ask about it at interview.", gap, reach.label(), iv.start.label())})
		}
		if iv.end > reach {
			reach = iv.end
		}
	}
	if gap := int(cur-reach) - 1; gap > 12 {
		flags = append(flags, Flag{"info", "gap",
			fmt.Sprintf("No role listed for the last %d months (since %s).", gap, reach.label())})
	}
	return flags
}

// TimelineSummary renders the computed timeline for the scoring prompt.
func (p Profile) TimelineSummary(now time.Time) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Computed by the system from the CV's dates, as of %s. Treat these durations as correct; do not recalculate them.\n", now.Format("2 January 2006"))
	if len(p.Employment) == 0 {
		sb.WriteString("- No employment history could be read from the CV.\n")
	}
	for _, r := range p.Employment {
		title := r.Title
		if title == "" {
			title = "(untitled role)"
		}
		ft := ""
		if r.FullTime != nil && !*r.FullTime {
			ft = " [not full-time]"
		}
		if r.DatesValid {
			fmt.Fprintf(&sb, "- %s — %s: %s to %s (%s)%s\n", title, orDash(r.Employer), r.StartLabel, r.EndLabel, FormatMonths(r.DurationMonths), ft)
		} else {
			fmt.Fprintf(&sb, "- %s — %s: dates missing or invalid%s\n", title, orDash(r.Employer), ft)
		}
	}
	fmt.Fprintf(&sb, "Total distinct time in employment (overlaps counted once): %s\n", FormatMonths(p.TotalMonths))
	return sb.String()
}

// FormatMonths renders 52 as "4 yrs 4 mos" and 13 as "1 yr 1 mo".
func FormatMonths(m int) string {
	y, mo := m/12, m%12
	unit := func(n int, one, many string) string {
		if n == 1 {
			return "1 " + one
		}
		return fmt.Sprintf("%d %s", n, many)
	}
	switch {
	case y == 0:
		return unit(mo, "mo", "mos")
	case mo == 0:
		return unit(y, "yr", "yrs")
	}
	return unit(y, "yr", "yrs") + " " + unit(mo, "mo", "mos")
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func maxM(a, b month) month {
	if a > b {
		return a
	}
	return b
}

func minM(a, b month) month {
	if a < b {
		return a
	}
	return b
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n...[truncated]"
}

// --- Demo mode -------------------------------------------------------------

var (
	reEmail    = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	rePhone    = regexp.MustCompile(`\+?\(?\d[\d\s\-()]{7,}\d`)
	reLinkedIn = regexp.MustCompile(`(?i)(https?://)?([a-z]{2,3}\.)?linkedin\.com/in/[A-Za-z0-9_\-%]+/?`)
	reRange    = regexp.MustCompile(`(?i)((?:jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)[a-z]*\.?\s+)?(\d{4})\s*(?:-|–|—|to|until)\s*((?:jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)[a-z]*\.?\s+)?(\d{4}|present|current)`)
)

var monthNames = map[string]int{"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6, "jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12}

// DemoHandler is a rough, regex-based stand-in for the AI extraction.
func DemoHandler(req llm.Request) (string, error) {
	text, _ := req.Hint.(string)
	p := Profile{}
	for _, line := range strings.Split(text, "\n") {
		if l := strings.TrimSpace(line); l != "" {
			p.Name = l
			break
		}
	}
	p.Email = reEmail.FindString(text)
	if ph := rePhone.FindString(text); ph != "" {
		p.Phone = strings.TrimSpace(ph)
	}
	p.LinkedInURL = reLinkedIn.FindString(text)
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		m := reRange.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		title := strings.TrimSpace(reRange.ReplaceAllString(line, ""))
		title = strings.Trim(title, " |,-–()")
		if title == "" && i > 0 {
			title = strings.TrimSpace(lines[i-1])
		}
		start := demoDate(m[1], m[2])
		end := demoDate(m[3], m[4])
		p.Employment = append(p.Employment, Role{Title: title, Start: start, End: end})
	}
	b, err := json.Marshal(p)
	return string(b), err
}

func demoDate(mon, year string) string {
	year = strings.ToLower(year)
	if year == "present" || year == "current" {
		return "present"
	}
	mon = strings.ToLower(strings.TrimSpace(mon))
	if len(mon) >= 3 {
		if n, ok := monthNames[mon[:3]]; ok {
			return fmt.Sprintf("%s-%02d", year, n)
		}
	}
	return year
}
