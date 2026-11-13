package export

import (
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"github.com/fmuoria/CV-Review-agent/internal/store"
)

func num(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

// JobReport writes the three-sheet workbook for one job.
func JobReport(w io.Writer, job store.Job, apps []store.Application, now time.Time) error {
	return Write(w, []Sheet{summarySheet(job, apps, now), rankedSheet(apps), detailSheet(apps)})
}

func summarySheet(job store.Job, apps []store.Application, now time.Time) Sheet {
	scored, total, strong := 0, 0.0, 0
	stage := map[string]int{}
	source := map[string]int{}
	for _, a := range apps {
		stage[a.Stage]++
		source[a.Source]++
		if a.ScoreTotal != nil {
			scored++
			total += *a.ScoreTotal
			if *a.ScoreTotal >= 70 {
				strong++
			}
		}
	}
	avg := 0.0
	if scored > 0 {
		avg = total / float64(scored)
	}
	rows := [][]Cell{
		{{V: "Candidate review — " + job.Title, Style: StyleTitle}},
		{{V: "Generated " + now.Format("2 Jan 2006 15:04 MST")}},
		{},
		{{V: "Applicants", Style: StyleBold}, {V: len(apps)}},
		{{V: "Screened by AI", Style: StyleBold}, {V: scored}},
		{{V: "Average score", Style: StyleBold}, {V: avg, Style: StyleNumber}},
		{{V: "Scoring 70 or more", Style: StyleBold}, {V: strong}},
		{},
		{{V: "By stage", Style: StyleHeader}, {V: "Count", Style: StyleHeader}},
	}
	for _, s := range store.Stages {
		if stage[s] > 0 {
			rows = append(rows, []Cell{{V: store.StageLabels[s]}, {V: stage[s]}})
		}
	}
	rows = append(rows, []Cell{}, []Cell{{V: "By source", Style: StyleHeader}, {V: "Count", Style: StyleHeader}})
	for _, s := range store.Sources {
		if source[s.Value] > 0 {
			rows = append(rows, []Cell{{V: s.Label}, {V: source[s.Value]}})
		}
	}
	rows = append(rows, []Cell{},
		[]Cell{{V: "Scoring rubric: Experience 0-50, Education 0-20, Duties 0-20, Cover letter 0-10. Green ≥ 70, amber 55-69, red < 55."}},
		[]Cell{{V: "AI scores rank applicants for human review. They are not hiring decisions."}})
	return Sheet{Name: "Summary", ColWidths: []float64{34, 14}, Rows: rows}
}

func rankedSheet(apps []store.Application) Sheet {
	header := []string{"Rank", "Name", "Email", "Phone", "Source", "Stage", "Total", "Experience /50", "Education /20",
		"Duties /20", "Cover letter /10", "Match", "Warnings", "Experience (yrs)"}
	rows := [][]Cell{headerRow(header)}
	for i, a := range apps {
		style := StyleNumber
		band := ""
		if a.ScoreTotal != nil {
			style = ScoreStyle(*a.ScoreTotal)
		}
		if a.Scoring != nil {
			band = a.Scoring.Band
		} else {
			band = statusText(a.ScreeningStatus)
		}
		years := any(nil)
		if a.Profile != nil {
			years = math.Round(float64(a.Profile.TotalMonths)/12*10) / 10
		}
		rows = append(rows, []Cell{
			{V: i + 1}, {V: a.Candidate.Name}, {V: a.Candidate.Email}, {V: a.Candidate.Phone},
			{V: store.SourceLabel(a.Source)}, {V: store.StageLabels[a.Stage]},
			{V: num(a.ScoreTotal), Style: style}, {V: num(a.ScoreExperience), Style: StyleNumber},
			{V: num(a.ScoreEducation), Style: StyleNumber}, {V: num(a.ScoreDuties), Style: StyleNumber},
			{V: num(a.ScoreCover), Style: StyleNumber}, {V: band}, {V: a.WarningCount()}, {V: years, Style: StyleNumber},
		})
	}
	return Sheet{Name: "Ranked Candidates", FreezeRows: 1,
		ColWidths: []float64{6, 24, 28, 16, 16, 12, 8, 13, 13, 11, 14, 15, 10, 14}, Rows: rows}
}

func detailSheet(apps []store.Application) Sheet {
	header := []string{"Rank", "Name", "Total", "Summary", "Experience reasoning", "Education reasoning", "Duties reasoning",
		"Cover letter reasoning", "Strengths", "Gaps to probe", "Required items not evidenced", "Consistency flags"}
	rows := [][]Cell{headerRow(header)}
	for i, a := range apps {
		r := a.Scoring
		if r == nil {
			rows = append(rows, []Cell{{V: i + 1}, {V: a.Candidate.Name}, {}, {V: statusText(a.ScreeningStatus) + " " + a.ScreeningError, Style: StyleWrap}})
			continue
		}
		var missing []string
		for _, c := range r.RequiredChecks {
			if c.Met == "no" {
				missing = append(missing, c.Requirement)
			}
		}
		var flags []string
		for _, f := range a.Flags {
			flags = append(flags, f.Message)
		}
		rows = append(rows, []Cell{
			{V: i + 1}, {V: a.Candidate.Name}, {V: r.Total, Style: ScoreStyle(r.Total)},
			{V: r.Summary, Style: StyleWrap}, {V: r.ExperienceReasoning, Style: StyleWrap}, {V: r.EducationReasoning, Style: StyleWrap},
			{V: r.DutiesReasoning, Style: StyleWrap}, {V: r.CoverLetterReasoning, Style: StyleWrap},
			{V: bullets(r.Strengths), Style: StyleWrap}, {V: bullets(r.Gaps), Style: StyleWrap},
			{V: bullets(missing), Style: StyleWrap}, {V: bullets(flags), Style: StyleWrap},
		})
	}
	return Sheet{Name: "Detailed Analysis", FreezeRows: 1,
		ColWidths: []float64{6, 22, 8, 45, 40, 35, 35, 35, 35, 35, 35, 40}, Rows: rows}
}

func headerRow(cols []string) []Cell {
	out := make([]Cell, len(cols))
	for i, c := range cols {
		out[i] = Cell{V: c, Style: StyleHeader}
	}
	return out
}

func bullets(xs []string) string {
	if len(xs) == 0 {
		return ""
	}
	return "• " + strings.Join(xs, "\n• ")
}

func statusText(s string) string {
	switch s {
	case "pending", "processing":
		return "Not yet screened"
	case "needs_review":
		return "Needs manual review"
	case "failed":
		return "Screening failed"
	}
	return fmt.Sprint(s)
}
