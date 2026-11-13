# Phase 1: from CV Review Agent to Sinapis Hiring

## What was kept

- **The scoring rubric and prompt** (`internal/scoring`): Experience 0–50, Education 0–20, Duties 0–20,
  Cover letter 0–10; required vs nice-to-have weighting; title-plus-duties validation; "keyword in the
  wrong context is not experience"; quantified-achievement matching; "do not guess missing information".
- **Ranking and tie-breakers**: total, then experience, duties, education, cover letter.
- **Gmail intake, bulk upload with `Name_CV` / `Name_CoverLetter` pairing, retry with backoff, Excel export**
  (Summary, Ranked Candidates, Detailed Analysis, colour-coded).
- **Gemini 2.5 Flash on Vertex AI** as the default model.

## What was fixed

| # | Problem in the desktop agent | Fix |
|---|---|---|
| 1 | Current date hardcoded as 22 Nov 2025 — "Present" roles were a year short and roles started after Nov 2025 were flagged as future dates | Date comes from the clock; durations are computed in Go (`internal/profile`) and given to the model as facts |
| 2 | Scores weren't capped; prompt "bonuses" could push totals over 100 | Every component is clamped to its range and the total recomputed; clamping is recorded |
| 3 | `condenseRequirements` sent only the first 3 required items | All required items are sent; nice-to-have lists capped at 10 |
| 4 | Applicants named from file names / email display names; same names overwrote each other; nested Gmail attachments skipped | Applicants keyed by email address; Gmail parts walked recursively; email body used as the cover letter when none is attached |
| 5 | Full model responses (candidate data) written to logs | Only application IDs, timings and errors are logged; request logs never include query strings or form data |
| 6 | Scanned/image PDFs silently skipped | Gemini transcribes them; if that fails the application is marked "Needs manual review" |
| 7 | `cloud.google.com/go/vertexai/genai` was deprecated in June 2025 and scheduled for removal in June 2026 | Replaced by a small REST client (`internal/llm`) supporting Vertex AI and Gemini API keys |

Also new in the scorer: per-requirement evidence (`yes / partial / no` with a quote), strengths, gaps
to probe at interview, a summary for the hiring team, an instruction to ignore personal
characteristics, and a stored prompt version and model name with every score.

## What was replaced

- Desktop GUI (Fyne) and the single-batch HTTP API → a multi-user web app with a database.
- In-memory results → Postgres: jobs, candidates, applications, documents, notes, audit log.
- File-name conventions as the only intake → careers form, tracked links, upload, Gmail import.
- `excelize`, `docx` and Google SDK dependencies → standard library. The only third-party
  dependency is the Postgres driver (`github.com/lib/pq`).

## Decisions worth knowing

- **No automatic posting to LinkedIn or BrighterMonday.** LinkedIn's Job Posting API isn't accepting new
  partners and its User Agreement forbids bots; BrighterMonday has no public API. The app generates
  the text and a tracked apply link per site instead.
- **No LinkedIn or social-media scraping.** Candidates can attach their LinkedIn "Save to PDF" export
  on the form; comparing it with the CV is planned for Phase 2, along with grounded Google search for
  shortlisted candidates.
- **Screening never changes a candidate's stage.** Kenya's Data Protection Act (s.35) gives people the
  right not to be subject to decisions based solely on automated processing. Every stage change is a
  person's action, stored with their name and (for rejections) a reason.
- **Timeline flags never affect the score.** They're shown to reviewers as things to check.
