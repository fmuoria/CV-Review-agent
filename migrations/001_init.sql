-- Sinapis ATS — initial schema.

CREATE TABLE users (
    id            BIGSERIAL PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE,
    name          TEXT NOT NULL DEFAULT '',
    role          TEXT NOT NULL CHECK (role IN ('admin', 'hr', 'hiring_manager')),
    active        BOOLEAN NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_login_at TIMESTAMPTZ
);

CREATE TABLE sessions (
    id         TEXT PRIMARY KEY,           -- random token (stored hashed would be better; see secure package)
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    csrf_token TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX sessions_user_idx ON sessions(user_id);

CREATE TABLE jobs (
    id                  BIGSERIAL PRIMARY KEY,
    slug                TEXT NOT NULL UNIQUE,
    title               TEXT NOT NULL,
    department          TEXT NOT NULL DEFAULT '',
    location            TEXT NOT NULL DEFAULT '',
    country             TEXT NOT NULL DEFAULT 'KE',
    employment_type     TEXT NOT NULL DEFAULT 'Full-time',
    role_family         TEXT NOT NULL DEFAULT 'general',
    summary             TEXT NOT NULL DEFAULT '',
    description         TEXT NOT NULL DEFAULT '',
    required_experience JSONB NOT NULL DEFAULT '[]',
    required_education  JSONB NOT NULL DEFAULT '[]',
    required_duties     JSONB NOT NULL DEFAULT '[]',
    nice_experience     JSONB NOT NULL DEFAULT '[]',
    nice_education      JSONB NOT NULL DEFAULT '[]',
    nice_duties         JSONB NOT NULL DEFAULT '[]',
    questions           JSONB NOT NULL DEFAULT '[]',  -- extra application questions shown on the form
    closes_on           DATE,
    status              TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'open', 'closed')),
    created_by          BIGINT REFERENCES users(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Hiring managers only see jobs they are members of. HR and admins see all jobs.
CREATE TABLE job_members (
    job_id  BIGINT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (job_id, user_id)
);

-- One row per person, shared across jobs (future talent pool).
CREATE TABLE candidates (
    id           BIGSERIAL PRIMARY KEY,
    email        TEXT UNIQUE,                 -- lower-cased; NULL when a CV had no email
    name         TEXT NOT NULL DEFAULT '',
    phone        TEXT NOT NULL DEFAULT '',
    location     TEXT NOT NULL DEFAULT '',
    linkedin_url TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE applications (
    id                BIGSERIAL PRIMARY KEY,
    job_id            BIGINT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    candidate_id      BIGINT NOT NULL REFERENCES candidates(id) ON DELETE CASCADE,
    source            TEXT NOT NULL DEFAULT 'website',  -- website, linkedin, brightermonday, email, referral, upload, other
    source_ref        TEXT NOT NULL DEFAULT '',         -- e.g. Gmail message id, for de-duplication
    stage             TEXT NOT NULL DEFAULT 'applied'
                      CHECK (stage IN ('applied','screened','shortlisted','interview','offer','hired','rejected','withdrawn')),
    answers           JSONB NOT NULL DEFAULT '[]',
    consent_at        TIMESTAMPTZ,                      -- when the candidate accepted the privacy notice
    cover_text        TEXT NOT NULL DEFAULT '',         -- cover letter typed into the form or email body

    -- Screening (AI) results
    screening_status  TEXT NOT NULL DEFAULT 'pending'
                      CHECK (screening_status IN ('pending','processing','done','failed','needs_review')),
    screening_error   TEXT NOT NULL DEFAULT '',
    screening_attempts INT NOT NULL DEFAULT 0,
    screening_started_at TIMESTAMPTZ,
    score_total       NUMERIC(5,2),
    score_experience  NUMERIC(5,2),
    score_education   NUMERIC(5,2),
    score_duties      NUMERIC(5,2),
    score_cover       NUMERIC(5,2),
    scoring           JSONB,      -- reasoning, per-requirement evidence, strengths, gaps
    profile           JSONB,      -- structured CV extraction (contacts, timeline, education)
    flags             JSONB NOT NULL DEFAULT '[]',  -- timeline/consistency flags computed in Go
    scored_at         TIMESTAMPTZ,
    model             TEXT NOT NULL DEFAULT '',
    prompt_version    TEXT NOT NULL DEFAULT '',

    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (job_id, candidate_id)
);
CREATE INDEX applications_job_idx ON applications(job_id, stage);
CREATE INDEX applications_screening_idx ON applications(screening_status) WHERE screening_status IN ('pending','processing');
CREATE UNIQUE INDEX applications_source_ref_idx ON applications(job_id, source_ref) WHERE source_ref <> '';

CREATE TABLE documents (
    id             BIGSERIAL PRIMARY KEY,
    application_id BIGINT NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    kind           TEXT NOT NULL CHECK (kind IN ('cv','cover_letter','linkedin_pdf','other')),
    filename       TEXT NOT NULL,
    content_type   TEXT NOT NULL DEFAULT '',
    size_bytes     BIGINT NOT NULL DEFAULT 0,
    storage_key    TEXT NOT NULL,
    text_content   TEXT NOT NULL DEFAULT '',
    extraction     TEXT NOT NULL DEFAULT '',   -- how text was obtained: plain, pdftotext, docx, ai_ocr, failed
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX documents_application_idx ON documents(application_id);

-- Append-only audit log: who did what, when. Required for human-in-the-loop accountability.
CREATE TABLE events (
    id             BIGSERIAL PRIMARY KEY,
    application_id BIGINT REFERENCES applications(id) ON DELETE CASCADE,
    job_id         BIGINT REFERENCES jobs(id) ON DELETE CASCADE,
    actor_id       BIGINT REFERENCES users(id),
    kind           TEXT NOT NULL,       -- applied, stage_changed, screened, screening_failed, note_added, rescreen_requested, ...
    detail         JSONB NOT NULL DEFAULT '{}',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX events_application_idx ON events(application_id, created_at);

CREATE TABLE notes (
    id             BIGSERIAL PRIMARY KEY,
    application_id BIGINT NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    author_id      BIGINT NOT NULL REFERENCES users(id),
    body           TEXT NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Organisation-wide integrations, e.g. the careers@ Gmail inbox. Secrets are encrypted at rest.
CREATE TABLE integrations (
    name            TEXT PRIMARY KEY,   -- 'gmail'
    account_email   TEXT NOT NULL DEFAULT '',
    secret_cipher   TEXT NOT NULL DEFAULT '',
    connected_by    BIGINT REFERENCES users(id),
    connected_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
