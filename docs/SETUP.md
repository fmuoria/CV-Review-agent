# Setup guide

Follow the parts in order. Parts A–C can be done today; Part D puts it online.
Commands are for **Git Bash on Windows** (they also work on macOS/Linux).

---

## Part A — Run it on your laptop (demo AI, ~15 minutes)

1. **Install Go 1.24+** from https://go.dev/dl/ and check:
   ```bash
   go version
   ```
2. **Get Postgres.** Easiest is Docker Desktop. Otherwise install PostgreSQL 16 for Windows and create a database:
   ```bash
   psql -U postgres -c "CREATE USER ats WITH PASSWORD 'ats' CREATEDB;"
   psql -U postgres -c "CREATE DATABASE ats OWNER ats;"
   psql -U postgres -c "CREATE DATABASE ats_test OWNER ats;"
   ```
3. **Check out the branch and run the tests:**
   ```bash
   git fetch origin && git checkout phase1-ats
   go mod download
   go test ./...
   TEST_DATABASE_URL="postgres://ats:ats@localhost:5432/ats_test?sslmode=disable" go test ./...
   ```
4. **Start the app:**
   ```bash
   cp .env.example .env
   set -a; source .env; set +a      # loads .env into this Git Bash session
   go run ./cmd/server
   ```
   Or with Docker: `docker compose up --build`.
5. Open http://localhost:8080 → **Development sign-in** → your email, role Admin.
6. Try it: **New job** → load the *MEL* template → status *Open* → **Create job** → **Upload CVs** → select the
   four files in `examples/`. Scores appear within seconds (demo mode = keyword matching).

> PDFs need `pdftotext`. Without it locally, PDFs are sent to the AI (or marked "needs review" in demo mode).
> On Windows you can install it with `pacman -S mingw-w64-x86_64-poppler` (MSYS2) or skip it. The Docker image includes it.

---

## Part B — Google Cloud for Gemini (~20 minutes)

Use a **Sinapis-owned** Google Cloud project with Sinapis billing, so the data and the bill belong to the organisation.

1. https://console.cloud.google.com → project picker → **New project** → name `sinapis-ats`. Note the **Project ID**.
2. **Enable APIs** (APIs & Services → Library): *Vertex AI API* and *Gmail API*.
   Also enable *Cloud Storage* if you plan to use `STORAGE_BACKEND=gcs`.
3. **Service account** (IAM & Admin → Service accounts → Create):
   - Name: `ats-screening`
   - Role: **Vertex AI User**. Add **Storage Object Admin** only if you'll use GCS.
4. Open the service account → **Keys** → *Add key* → *JSON*. A file downloads. Treat it like a password:
   don't email it and don't commit it to git.
   - If key creation is blocked, your organisation policy `iam.disableServiceAccountKeyCreation` is on.
     Ask the Workspace/Cloud admin for an exception for this project.
5. **Test Gemini locally:** in `.env` set
   ```
   LLM_PROVIDER=vertex
   GOOGLE_CLOUD_PROJECT=<your project id>
   GOOGLE_APPLICATION_CREDENTIALS=C:/path/to/key.json
   ```
   Restart the app. On a job, click **Re-screen all applicants**. The demo banner disappears and the scores now come from Gemini.

> **Region:** the default `GOOGLE_CLOUD_LOCATION=us-central1` matches the old desktop agent.
> To keep processing in Europe, next to the Render servers, use `europe-west1` (check Gemini 2.5 Flash is listed for that region in Vertex AI).
> Vertex AI does not use your prompts to train Google's models.

---

## Part C — Google sign-in and Gmail access (~15 minutes)

1. Cloud console → **Google Auth Platform** (or *APIs & Services → OAuth consent screen*):
   - **Audience / User type: Internal.** Only @sinapis.org accounts can use it, and Google doesn't need to review the app.
   - App name: *Sinapis Hiring*. Support email: yours.
   - **Data access / Scopes:** add `openid`, `email`, `profile` and `https://www.googleapis.com/auth/gmail.readonly`.
2. **Clients → Create client → Web application.** Name: *Sinapis Hiring*. Under **Authorized redirect URIs** add:
   ```
   http://localhost:8080/auth/callback
   http://localhost:8080/settings/gmail/callback
   ```
   Add the production URLs in Part D, once you know them.
3. Copy the **Client ID** and **Client secret** into `.env`:
   ```
   GOOGLE_CLIENT_ID=...
   GOOGLE_CLIENT_SECRET=...
   ALLOWED_EMAIL_DOMAIN=sinapis.org
   ADMIN_EMAILS=francis@sinapis.org
   ```
4. Restart the app → **Sign in with Google**. The `ADMIN_EMAILS` accounts become admins on first sign-in.
   Everyone else must be added under **People** first.

---

## Part D — Deploy on Render (~30 minutes)

1. Push the branch to GitHub (or merge it into `main`).
2. Render dashboard → **New → Blueprint** → choose the repo/branch.
   Render reads `render.yaml` and proposes:
   - a web service `sinapis-ats` (Docker, Frankfurt, Starter plan, 5 GB disk)
   - a Postgres database `sinapis-ats-db`
3. Fill in the values marked "sync: false":

   | Variable | Value |
   |---|---|
   | `BASE_URL` | `https://sinapis-ats.onrender.com`. Use your real URL; you can change it later |
   | `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` | from Part C |
   | `ADMIN_EMAILS` | e.g. `francis@sinapis.org,hr@sinapis.org` |
   | `GOOGLE_CLOUD_PROJECT` | from Part B |
   | `GOOGLE_SERVICE_ACCOUNT_JSON` | open the key file and paste the **entire JSON** |
   | `PRIVACY_CONTACT_EMAIL` | e.g. `privacy@sinapis.org` (shown to applicants) |

   `APP_SECRET` is generated for you. **Don't change it later**: it encrypts the stored Gmail connection.
4. **Apply.** The first build takes a few minutes. Check *Logs* for `listening`.
5. Back in **Google Auth Platform → Clients**, add the production redirect URIs:
   ```
   https://<your-render-url>/auth/callback
   https://<your-render-url>/settings/gmail/callback
   ```
6. Open the site → **Sign in with Google** → **People** → add HR (role *HR*) and hiring managers (role *Hiring manager*).

### Connect the applications inbox
**Settings → Connect Gmail.** Choose the inbox that receives applications, for example `careers@sinapis.org`.
You need to be able to sign in as that account. Access is read-only. Then on a job: **Import from Gmail**.

### Put it on sinapis.org
- Simplest: add a *Careers* link on the website pointing to `https://<your-url>/careers`.
- Nicer: in Render → service → **Settings → Custom domains**, add `careers.sinapis.org` and create the DNS record Render shows.
  Then update `BASE_URL` and the two OAuth redirect URIs to the new domain.

---

## Part E — Running it

- **Posting a job:** job page → **Post the job**. Copy the text and the channel's apply link into LinkedIn and BrighterMonday.
  Every applicant is tagged with where they came from.
- **BrighterMonday CVs** that arrive inside BrighterMonday: download them → **Upload CVs** → source *BrighterMonday*.
- **Changing requirements after applications arrive:** edit the job, then **Re-screen all applicants** so the scores match.
- **Costs:**
  - Render: Starter web service + `basic-256mb` Postgres + 5 GB disk.
  - Vertex AI: billed per token.
  - Each applicant uses two Gemini calls (profile + score), plus one more for a scanned CV.
  - Check the current Render and Vertex AI pricing pages, and set a budget alert in Google Cloud billing.
- **Backups:** check which backup options your Render Postgres plan includes, and turn them on.
  CV files live on the Render disk. For off-site copies, switch to `STORAGE_BACKEND=gcs` with a private bucket.
  Switch before you have real data: existing files aren't migrated automatically.

## Troubleshooting

| Symptom | Likely cause |
|---|---|
| App won't start: "configuration problems" | The message lists the missing variables. In production `APP_SECRET`, Google sign-in and a non-demo AI provider are required |
| "redirect_uri_mismatch" from Google | The OAuth client's redirect URIs don't exactly match `BASE_URL` + `/auth/callback` |
| "hasn't been given access yet" | Add the person under **People**, or put them in `ADMIN_EMAILS` |
| Applications stuck on "Waiting" | Check *Logs* for `screening failed`. Usually the service account lacks *Vertex AI User*, or the Vertex AI API isn't enabled |
| "Gmail access was refused" | Reconnect the inbox in Settings. Google revokes tokens after a password change |
