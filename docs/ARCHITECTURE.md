# How it fits together

```
 Careers form ─┐                       ┌─ pdftotext / DOCX reader
 Bulk upload  ─┼─► intake ─► Postgres ─┤                        ┌─► profile + timeline checks (Go)
 Gmail import ─┘   (files to disk/GCS)  └─ screening worker ──────┼─► Gemini: structured CV
                                           (queue in DB)          └─► Gemini: score + evidence
                                                │
                       Dashboard (HR / hiring managers) ◄────────┘
                       ranked list · compare · stages · notes · export
```

**One process.** `cmd/server` runs the web server and the screening workers together. The queue
lives in the `applications` table (`screening_status`), claimed with `FOR UPDATE SKIP LOCKED`, so it
survives restarts and you could run several instances later.

**Screening steps** (`internal/screening`):
1. Extract text from each document (cached in `documents.text_content`).
2. Ask Gemini for a structured profile; compute durations, overlaps, gaps and future dates in Go.
3. Fill blank candidate details (name, phone, LinkedIn) from the profile.
4. Score with the rubric; clamp; save scores, evidence, profile and flags.
5. Failures retry up to 3 times; unreadable CVs go to "Needs manual review".

**Security.**
- Google sign-in restricted to the configured domain and to invited users.
- Server-side sessions with CSRF tokens on every form.
- Strict Content-Security-Policy (no inline scripts or styles).
- Files are served only to people with access to the job, as downloads.
- The Gmail refresh token is encrypted with AES-GCM using a key derived from `APP_SECRET`.
- The public form has a honeypot field and a signed timing token against spam.

**Data protection.**
- The careers form shows a privacy notice and records consent.
- The audit log records every stage change, re-screen, export and import.
- Applications added by HR or imported from email are marked "privacy notice not recorded".
