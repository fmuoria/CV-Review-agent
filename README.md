# Sinapis Hiring (ATS)

A web-based applicant tracking system for Sinapis, built on the
[CV Review Agent](https://github.com/fmuoria/CV-Review-agent) scoring engine.

HR and hiring managers create jobs, collect applications from the careers page, LinkedIn,
BrighterMonday and email, and get every applicant read and ranked by Gemini against the job's
requirements — with the evidence for each score. People make every decision; the app records who
did what.

## What it does (Phase 1)

| Area | What you get |
|---|---|
| **Jobs** | Role templates (Programme/BDS, Finance, MEL, Operations, Communications, Technology), required vs nice-to-have lists, extra application questions, closing dates, draft/open/closed |
| **Getting applicants** | Public careers page + application form with a privacy notice; one tracked apply link per channel (LinkedIn, BrighterMonday, website, referral); bulk CV upload; Gmail inbox import |
| **AI screening** | Background queue: text extraction (scanned PDFs read by Gemini), structured CV profile, timeline checks done in code, scoring with the original rubric — Experience 50, Education 20, Duties 20, Cover letter 10 — plus per-requirement evidence, strengths and gaps |
| **Reviewing** | Ranked list with filters, pipeline stages, side-by-side compare, candidate page with evidence, timeline, documents, notes and full history, colour-coded Excel export |
| **Access** | Google sign-in limited to your domain; Admin / HR / Hiring manager roles; hiring managers only see their jobs |

What it deliberately does **not** do: post to LinkedIn or BrighterMonday automatically (neither
allows it for third-party apps), scrape LinkedIn or social media, or reject anyone automatically.
See `docs/CHANGES.md` for why.

## Quick start (no Google account needed)

```bash
cp .env.example .env          # LLM_PROVIDER=demo uses keyword matching instead of Gemini
docker compose up --build     # Postgres + app
# open http://localhost:8080 and use "Development sign-in"
```

Without Docker: install Postgres, create a database, then

```bash
export DATABASE_URL="postgres://USER:PASS@localhost:5432/ats?sslmode=disable"
go run ./cmd/server
```

Full setup — Google Cloud, sign-in, Render deployment — is in **[docs/SETUP.md](docs/SETUP.md)**.

## Project layout

```
cmd/server          entry point: web server + screening workers
migrations          SQL schema (applied automatically on start)
internal/
  config            environment settings
  llm               Gemini REST client (Vertex AI or API key) + demo client
  googleauth        service-account tokens without the Google SDKs
  extract           PDF / Word / image → text (pdftotext, then Gemini for scans)
  profile           structured CV extraction + timeline checks (overlaps, gaps, future dates)
  scoring           the scoring prompt and rubric (from the desktop agent, fixed)
  screening         background queue that screens applications
  intake            one validated path for form, upload and Gmail applications
  gmail             read-only Gmail import
  store             database access
  storage           file storage (Render disk or Google Cloud Storage)
  export            Excel workbook
  posting           per-channel job ad text and tracked links
  templates         role templates
  web               handlers, HTML templates, CSS/JS
docs/               setup guide, architecture, change log
```

## Tests

```bash
go test ./...                                                          # unit tests
TEST_DATABASE_URL="postgres://ats:ats@localhost:5432/ats_test?sslmode=disable" go test ./...   # + database tests
```

The database tests wipe the database they're given — use a separate test database.
