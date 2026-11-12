package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/fmuoria/CV-Review-agent/internal/db"
	"github.com/fmuoria/CV-Review-agent/internal/profile"
	"github.com/fmuoria/CV-Review-agent/internal/scoring"
)

// Store wraps the database connection.
type Store struct{ DB *sql.DB }

// New returns a Store.
func New(conn *sql.DB) *Store { return &Store{DB: conn} }

func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func toJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("null")
	}
	return b
}

func emptyIfNil(xs []string) []string {
	if xs == nil {
		return []string{}
	}
	return xs
}

// ---------------------------------------------------------------- users

const userCols = `id, email, name, role, active, created_at, last_login_at`

func scanUser(row interface{ Scan(...any) error }) (User, error) {
	var u User
	var last sql.NullTime
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.Active, &u.CreatedAt, &last)
	if last.Valid {
		u.LastLoginAt = &last.Time
	}
	return u, err
}

// GetUserByEmail looks a user up by email (case-insensitive).
func (s *Store) GetUserByEmail(ctx context.Context, email string) (User, error) {
	u, err := scanUser(s.DB.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE email=$1`, strings.ToLower(email)))
	return u, notFound(err)
}

// GetUser loads a user by id.
func (s *Store) GetUser(ctx context.Context, id int64) (User, error) {
	u, err := scanUser(s.DB.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id=$1`, id))
	return u, notFound(err)
}

// CreateUser adds a staff member.
func (s *Store) CreateUser(ctx context.Context, email, name, role string) (User, error) {
	u, err := scanUser(s.DB.QueryRowContext(ctx,
		`INSERT INTO users(email, name, role) VALUES ($1,$2,$3) RETURNING `+userCols,
		strings.ToLower(strings.TrimSpace(email)), strings.TrimSpace(name), role))
	return u, err
}

// UpdateUser changes a user's role, active flag and name.
func (s *Store) UpdateUser(ctx context.Context, id int64, name, role string, active bool) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE users SET name=$2, role=$3, active=$4 WHERE id=$1`, id, name, role, active)
	return err
}

// TouchLogin records a sign-in and fills in the name if it was blank.
func (s *Store) TouchLogin(ctx context.Context, id int64, name string) error {
	_, err := s.DB.ExecContext(ctx,
		`UPDATE users SET last_login_at=now(), name=CASE WHEN name='' THEN $2 ELSE name END WHERE id=$1`, id, name)
	return err
}

// ListUsers returns all staff accounts.
func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+userCols+` FROM users ORDER BY active DESC, role, email`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- sessions

// CreateSession starts a session and returns it.
func (s *Store) CreateSession(ctx context.Context, userID int64, ttl time.Duration) (Session, error) {
	sess := Session{ID: randomToken(32), UserID: userID, CSRFToken: randomToken(24), ExpiresAt: time.Now().Add(ttl)}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO sessions(id, user_id, csrf_token, expires_at) VALUES ($1,$2,$3,$4)`,
		sess.ID, sess.UserID, sess.CSRFToken, sess.ExpiresAt)
	return sess, err
}

// GetSession returns a live session and its (active) user.
func (s *Store) GetSession(ctx context.Context, id string) (Session, User, error) {
	var sess Session
	var u User
	var last sql.NullTime
	err := s.DB.QueryRowContext(ctx, `
		SELECT s.id, s.user_id, s.csrf_token, s.expires_at,
		       u.id, u.email, u.name, u.role, u.active, u.created_at, u.last_login_at
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.id=$1 AND s.expires_at > now() AND u.active`, id).
		Scan(&sess.ID, &sess.UserID, &sess.CSRFToken, &sess.ExpiresAt,
			&u.ID, &u.Email, &u.Name, &u.Role, &u.Active, &u.CreatedAt, &last)
	if last.Valid {
		u.LastLoginAt = &last.Time
	}
	return sess, u, notFound(err)
}

// DeleteSession signs a session out.
func (s *Store) DeleteSession(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE id=$1`, id)
	return err
}

// DeleteExpiredSessions removes old sessions.
func (s *Store) DeleteExpiredSessions(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < now()`)
	return err
}

// ---------------------------------------------------------------- jobs

const jobCols = `id, slug, title, department, location, country, employment_type, role_family, summary, description,
	required_experience, required_education, required_duties, nice_experience, nice_education, nice_duties, questions,
	closes_on, status, created_by, created_at, updated_at`

func scanJob(row interface{ Scan(...any) error }) (Job, error) {
	var j Job
	var re, red, rd, ne, ned, nd, q []byte
	var closes sql.NullTime
	var createdBy sql.NullInt64
	err := row.Scan(&j.ID, &j.Slug, &j.Title, &j.Department, &j.Location, &j.Country, &j.EmploymentType, &j.RoleFamily,
		&j.Summary, &j.Description, &re, &red, &rd, &ne, &ned, &nd, &q, &closes, &j.Status, &createdBy, &j.CreatedAt, &j.UpdatedAt)
	if err != nil {
		return j, err
	}
	for _, p := range []struct {
		raw []byte
		dst *[]string
	}{{re, &j.RequiredExperience}, {red, &j.RequiredEducation}, {rd, &j.RequiredDuties},
		{ne, &j.NiceExperience}, {ned, &j.NiceEducation}, {nd, &j.NiceDuties}, {q, &j.Questions}} {
		_ = json.Unmarshal(p.raw, p.dst)
	}
	if closes.Valid {
		j.ClosesOn = &closes.Time
	}
	if createdBy.Valid {
		j.CreatedBy = &createdBy.Int64
	}
	return j, nil
}

// SaveJob inserts (ID==0) or updates a job. A unique slug is generated on insert.
func (s *Store) SaveJob(ctx context.Context, j *Job) error {
	args := []any{j.Title, j.Department, j.Location, j.Country, j.EmploymentType, j.RoleFamily, j.Summary, j.Description,
		toJSON(emptyIfNil(j.RequiredExperience)), toJSON(emptyIfNil(j.RequiredEducation)), toJSON(emptyIfNil(j.RequiredDuties)),
		toJSON(emptyIfNil(j.NiceExperience)), toJSON(emptyIfNil(j.NiceEducation)), toJSON(emptyIfNil(j.NiceDuties)),
		toJSON(emptyIfNil(j.Questions)), j.ClosesOn, j.Status}
	if j.ID != 0 {
		_, err := s.DB.ExecContext(ctx, `UPDATE jobs SET title=$1, department=$2, location=$3, country=$4, employment_type=$5,
			role_family=$6, summary=$7, description=$8, required_experience=$9, required_education=$10, required_duties=$11,
			nice_experience=$12, nice_education=$13, nice_duties=$14, questions=$15, closes_on=$16, status=$17, updated_at=now()
			WHERE id=$18`, append(args, j.ID)...)
		return err
	}
	base := slugify(j.Title)
	for attempt := 0; attempt < 20; attempt++ {
		slug := base
		if attempt > 0 {
			slug = fmt.Sprintf("%s-%s", base, strings.ToLower(randomToken(3)))
		}
		err := s.DB.QueryRowContext(ctx, `INSERT INTO jobs(title, department, location, country, employment_type, role_family,
			summary, description, required_experience, required_education, required_duties, nice_experience, nice_education,
			nice_duties, questions, closes_on, status, slug, created_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19) RETURNING id, slug, created_at, updated_at`,
			append(args, slug, j.CreatedBy)...).Scan(&j.ID, &j.Slug, &j.CreatedAt, &j.UpdatedAt)
		if db.IsUniqueViolation(err) {
			continue
		}
		return err
	}
	return errors.New("could not generate a unique job slug")
}

func slugify(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 60 {
		out = strings.Trim(out[:60], "-")
	}
	if out == "" {
		out = "job"
	}
	return out
}

// SetJobStatus opens, closes or drafts a job.
func (s *Store) SetJobStatus(ctx context.Context, id int64, status string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE jobs SET status=$2, updated_at=now() WHERE id=$1`, id, status)
	return err
}

// GetJob loads a job by id.
func (s *Store) GetJob(ctx context.Context, id int64) (Job, error) {
	j, err := scanJob(s.DB.QueryRowContext(ctx, `SELECT `+jobCols+` FROM jobs WHERE id=$1`, id))
	return j, notFound(err)
}

// GetJobBySlug loads a job by its public slug.
func (s *Store) GetJobBySlug(ctx context.Context, slug string) (Job, error) {
	j, err := scanJob(s.DB.QueryRowContext(ctx, `SELECT `+jobCols+` FROM jobs WHERE slug=$1`, slug))
	return j, notFound(err)
}

// ListJobs returns the jobs a user may see, with pipeline counts.
func (s *Store) ListJobs(ctx context.Context, u User) ([]Job, error) {
	q := `SELECT ` + jobCols + ` FROM jobs`
	args := []any{}
	if !u.CanManageJobs() {
		q += ` WHERE id IN (SELECT job_id FROM job_members WHERE user_id=$1)`
		args = append(args, u.ID)
	}
	q += ` ORDER BY CASE status WHEN 'open' THEN 0 WHEN 'draft' THEN 1 ELSE 2 END, created_at DESC`
	jobs, err := s.queryJobs(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	return jobs, s.fillCounts(ctx, jobs)
}

// ListPublicJobs returns open jobs for the careers page.
func (s *Store) ListPublicJobs(ctx context.Context) ([]Job, error) {
	return s.queryJobs(ctx, `SELECT `+jobCols+` FROM jobs WHERE status='open' AND (closes_on IS NULL OR closes_on >= CURRENT_DATE) ORDER BY created_at DESC`)
}

func (s *Store) queryJobs(ctx context.Context, q string, args ...any) ([]Job, error) {
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

func (s *Store) fillCounts(ctx context.Context, jobs []Job) error {
	if len(jobs) == 0 {
		return nil
	}
	idx := map[int64]int{}
	ids := make([]int64, len(jobs))
	for i := range jobs {
		jobs[i].StageCounts = map[string]int{}
		idx[jobs[i].ID] = i
		ids[i] = jobs[i].ID
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT job_id, stage, count(*),
		count(*) FILTER (WHERE screening_status IN ('pending','processing'))
		FROM applications WHERE job_id = ANY($1) GROUP BY job_id, stage`, pq.Array(ids))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var stage string
		var n, pending int
		if err := rows.Scan(&id, &stage, &n, &pending); err != nil {
			return err
		}
		j := &jobs[idx[id]]
		j.StageCounts[stage] = n
		j.Total += n
		j.PendingCount += pending
	}
	return rows.Err()
}

// CanAccessJob reports whether the user may see a job's candidates.
func (s *Store) CanAccessJob(ctx context.Context, u User, jobID int64) (bool, error) {
	if u.CanManageJobs() {
		return true, nil
	}
	var ok bool
	err := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM job_members WHERE job_id=$1 AND user_id=$2)`, jobID, u.ID).Scan(&ok)
	return ok, err
}

// JobMembers returns the user ids assigned to a job.
func (s *Store) JobMembers(ctx context.Context, jobID int64) (map[int64]bool, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT user_id FROM job_members WHERE job_id=$1`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// SetJobMembers replaces the hiring managers assigned to a job.
func (s *Store) SetJobMembers(ctx context.Context, jobID int64, userIDs []int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM job_members WHERE job_id=$1`, jobID); err != nil {
		return err
	}
	for _, uid := range userIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO job_members(job_id, user_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, jobID, uid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---------------------------------------------------------------- candidates & applications

// NewApplication is the input for creating an application.
type NewApplication struct {
	JobID       int64
	Email       string
	Name        string
	Phone       string
	Location    string
	LinkedInURL string
	Source      string
	SourceRef   string
	Answers     []scoring.QA
	CoverText   string
	Consent     bool
}

// CreateApplication finds or creates the candidate (by email) and adds an
// application for the job. Returns ErrAlreadyApplied for duplicates.
func (s *Store) CreateApplication(ctx context.Context, in NewApplication) (int64, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	email := strings.ToLower(strings.TrimSpace(in.Email))
	var candID int64
	if email != "" {
		err = tx.QueryRowContext(ctx, `INSERT INTO candidates(email, name, phone, location, linkedin_url) VALUES ($1,$2,$3,$4,$5)
			ON CONFLICT (email) DO UPDATE SET
				name = CASE WHEN EXCLUDED.name <> '' THEN EXCLUDED.name ELSE candidates.name END,
				phone = CASE WHEN EXCLUDED.phone <> '' THEN EXCLUDED.phone ELSE candidates.phone END,
				location = CASE WHEN EXCLUDED.location <> '' THEN EXCLUDED.location ELSE candidates.location END,
				linkedin_url = CASE WHEN EXCLUDED.linkedin_url <> '' THEN EXCLUDED.linkedin_url ELSE candidates.linkedin_url END,
				updated_at = now()
			RETURNING id`, email, in.Name, in.Phone, in.Location, in.LinkedInURL).Scan(&candID)
	} else {
		err = tx.QueryRowContext(ctx, `INSERT INTO candidates(email, name, phone, location, linkedin_url) VALUES (NULL,$1,$2,$3,$4) RETURNING id`,
			in.Name, in.Phone, in.Location, in.LinkedInURL).Scan(&candID)
	}
	if err != nil {
		return 0, fmt.Errorf("save candidate: %w", err)
	}

	var consent *time.Time
	if in.Consent {
		now := time.Now()
		consent = &now
	}
	if in.Source == "" {
		in.Source = "website"
	}
	var appID int64
	err = tx.QueryRowContext(ctx, `INSERT INTO applications(job_id, candidate_id, source, source_ref, answers, consent_at, cover_text)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		in.JobID, candID, in.Source, in.SourceRef, toJSON(nonNilQA(in.Answers)), consent, in.CoverText).Scan(&appID)
	if db.IsUniqueViolation(err) {
		return 0, ErrAlreadyApplied
	}
	if err != nil {
		return 0, fmt.Errorf("save application: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO events(application_id, job_id, kind, detail) VALUES ($1,$2,'applied',$3)`,
		appID, in.JobID, toJSON(map[string]any{"source": in.Source})); err != nil {
		return 0, err
	}
	return appID, tx.Commit()
}

func nonNilQA(q []scoring.QA) []scoring.QA {
	if q == nil {
		return []scoring.QA{}
	}
	return q
}

// SourceRefExists reports whether an import (e.g. a Gmail message) was already processed for the job.
func (s *Store) SourceRefExists(ctx context.Context, jobID int64, ref string) (bool, error) {
	var ok bool
	err := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM applications WHERE job_id=$1 AND source_ref=$2)`, jobID, ref).Scan(&ok)
	return ok, err
}

const appCols = `a.id, a.job_id, j.title, a.candidate_id, COALESCE(c.email,''), c.name, c.phone, c.location, c.linkedin_url,
	a.source, a.source_ref, a.stage, a.answers, a.consent_at, a.cover_text,
	a.screening_status, a.screening_error, a.screening_attempts,
	a.score_total, a.score_experience, a.score_education, a.score_duties, a.score_cover,
	a.scoring, a.profile, a.flags, a.scored_at, a.model, a.prompt_version, a.created_at, a.updated_at`

const appFrom = ` FROM applications a JOIN candidates c ON c.id=a.candidate_id JOIN jobs j ON j.id=a.job_id`

func scanApp(row interface{ Scan(...any) error }) (Application, error) {
	var a Application
	var answers, scoringRaw, profileRaw, flags []byte
	var consent, scored sql.NullTime
	var tot, exp, edu, dut, cov sql.NullFloat64
	err := row.Scan(&a.ID, &a.JobID, &a.JobTitle, &a.CandidateID, &a.Candidate.Email, &a.Candidate.Name, &a.Candidate.Phone,
		&a.Candidate.Location, &a.Candidate.LinkedInURL, &a.Source, &a.SourceRef, &a.Stage, &answers, &consent, &a.CoverText,
		&a.ScreeningStatus, &a.ScreeningError, &a.ScreeningAttempts, &tot, &exp, &edu, &dut, &cov,
		&scoringRaw, &profileRaw, &flags, &scored, &a.Model, &a.PromptVersion, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return a, err
	}
	a.Candidate.ID = a.CandidateID
	_ = json.Unmarshal(answers, &a.Answers)
	_ = json.Unmarshal(flags, &a.Flags)
	if len(scoringRaw) > 0 && string(scoringRaw) != "null" {
		var r scoring.Result
		if json.Unmarshal(scoringRaw, &r) == nil {
			a.Scoring = &r
		}
	}
	if len(profileRaw) > 0 && string(profileRaw) != "null" {
		var p profile.Profile
		if json.Unmarshal(profileRaw, &p) == nil {
			a.Profile = &p
		}
	}
	if consent.Valid {
		a.ConsentAt = &consent.Time
	}
	if scored.Valid {
		a.ScoredAt = &scored.Time
	}
	for _, p := range []struct {
		src sql.NullFloat64
		dst **float64
	}{{tot, &a.ScoreTotal}, {exp, &a.ScoreExperience}, {edu, &a.ScoreEducation}, {dut, &a.ScoreDuties}, {cov, &a.ScoreCover}} {
		if p.src.Valid {
			v := p.src.Float64
			*p.dst = &v
		}
	}
	return a, nil
}

// GetApplication loads one application with its candidate.
func (s *Store) GetApplication(ctx context.Context, id int64) (Application, error) {
	a, err := scanApp(s.DB.QueryRowContext(ctx, `SELECT `+appCols+appFrom+` WHERE a.id=$1`, id))
	return a, notFound(err)
}

// rankOrder is the ranking used everywhere: total score, then the desktop
// agent's tie-breakers (experience, duties, education, cover letter).
const rankOrder = ` ORDER BY a.score_total DESC NULLS LAST, a.score_experience DESC NULLS LAST, a.score_duties DESC NULLS LAST,
	a.score_education DESC NULLS LAST, a.score_cover DESC NULLS LAST, a.created_at`

// ListApplications returns a job's applications, ranked.
func (s *Store) ListApplications(ctx context.Context, jobID int64, f ApplicationFilter) ([]Application, error) {
	q := `SELECT ` + appCols + appFrom + ` WHERE a.job_id=$1`
	args := []any{jobID}
	add := func(cond string, v any) {
		args = append(args, v)
		q += fmt.Sprintf(" AND "+cond, len(args))
	}
	if f.Stage == "active" {
		q += ` AND a.stage NOT IN ('rejected','withdrawn')`
	} else if f.Stage != "" {
		add("a.stage=$%d", f.Stage)
	}
	if f.Source != "" {
		add("a.source=$%d", f.Source)
	}
	if f.MinScore != nil {
		add("a.score_total >= $%d", *f.MinScore)
	}
	if f.Query != "" {
		add("(c.name ILIKE $%[1]d OR c.email ILIKE $%[1]d)", "%"+f.Query+"%")
	}
	if len(f.IDs) > 0 {
		add("a.id = ANY($%d)", pq.Array(f.IDs))
	}
	rows, err := s.DB.QueryContext(ctx, q+rankOrder, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Application
	for rows.Next() {
		a, err := scanApp(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// SetStage moves an application through the pipeline and logs who did it.
func (s *Store) SetStage(ctx context.Context, appID int64, stage string, actor int64, reason string) error {
	if !ValidStage(stage) {
		return fmt.Errorf("invalid stage %q", stage)
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var from string
	var jobID int64
	if err := tx.QueryRowContext(ctx, `SELECT stage, job_id FROM applications WHERE id=$1 FOR UPDATE`, appID).Scan(&from, &jobID); err != nil {
		return notFound(err)
	}
	if from == stage {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE applications SET stage=$2, updated_at=now() WHERE id=$1`, appID, stage); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO events(application_id, job_id, actor_id, kind, detail) VALUES ($1,$2,$3,'stage_changed',$4)`,
		appID, jobID, actor, toJSON(map[string]any{"from": from, "to": stage, "reason": reason})); err != nil {
		return err
	}
	return tx.Commit()
}

// AddEvent appends to the audit log.
func (s *Store) AddEvent(ctx context.Context, appID, jobID int64, actor *int64, kind string, detail map[string]any) error {
	var app, job any
	if appID != 0 {
		app = appID
	}
	if jobID != 0 {
		job = jobID
	}
	if detail == nil {
		detail = map[string]any{}
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO events(application_id, job_id, actor_id, kind, detail) VALUES ($1,$2,$3,$4,$5)`,
		app, job, actor, kind, toJSON(detail))
	return err
}

// ListEvents returns an application's history, oldest first.
func (s *Store) ListEvents(ctx context.Context, appID int64) ([]Event, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT e.id, e.kind, e.detail, e.created_at, COALESCE(NULLIF(u.name,''), u.email, '')
		FROM events e LEFT JOIN users u ON u.id=e.actor_id WHERE e.application_id=$1 ORDER BY e.created_at, e.id`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		var detail []byte
		if err := rows.Scan(&e.ID, &e.Kind, &detail, &e.CreatedAt, &e.ActorName); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(detail, &e.Detail)
		out = append(out, e)
	}
	return out, rows.Err()
}

// AddNote stores a comment on an application.
func (s *Store) AddNote(ctx context.Context, appID, author int64, body string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO notes(application_id, author_id, body) VALUES ($1,$2,$3)`, appID, author, body); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO events(application_id, actor_id, kind) VALUES ($1,$2,'note_added')`, appID, author); err != nil {
		return err
	}
	return tx.Commit()
}

// ListNotes returns an application's notes, newest first.
func (s *Store) ListNotes(ctx context.Context, appID int64) ([]Note, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT n.id, COALESCE(NULLIF(u.name,''), u.email), n.body, n.created_at
		FROM notes n JOIN users u ON u.id=n.author_id WHERE n.application_id=$1 ORDER BY n.created_at DESC`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Note
	for rows.Next() {
		var n Note
		if err := rows.Scan(&n.ID, &n.AuthorName, &n.Body, &n.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- documents

// AddDocument records an uploaded file.
func (s *Store) AddDocument(ctx context.Context, d *Document) error {
	return s.DB.QueryRowContext(ctx, `INSERT INTO documents(application_id, kind, filename, content_type, size_bytes, storage_key, text_content, extraction)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id, created_at`,
		d.ApplicationID, d.Kind, d.Filename, d.ContentType, d.SizeBytes, d.StorageKey, d.TextContent, d.Extraction).Scan(&d.ID, &d.CreatedAt)
}

// ListDocuments returns an application's files.
func (s *Store) ListDocuments(ctx context.Context, appID int64) ([]Document, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, application_id, kind, filename, content_type, size_bytes, storage_key, text_content, extraction, created_at
		FROM documents WHERE application_id=$1 ORDER BY CASE kind WHEN 'cv' THEN 0 WHEN 'cover_letter' THEN 1 ELSE 2 END, id`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Document
	for rows.Next() {
		var d Document
		if err := rows.Scan(&d.ID, &d.ApplicationID, &d.Kind, &d.Filename, &d.ContentType, &d.SizeBytes, &d.StorageKey, &d.TextContent, &d.Extraction, &d.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// GetDocument loads one file record.
func (s *Store) GetDocument(ctx context.Context, id int64) (Document, error) {
	var d Document
	err := s.DB.QueryRowContext(ctx, `SELECT id, application_id, kind, filename, content_type, size_bytes, storage_key, text_content, extraction, created_at
		FROM documents WHERE id=$1`, id).Scan(&d.ID, &d.ApplicationID, &d.Kind, &d.Filename, &d.ContentType, &d.SizeBytes, &d.StorageKey, &d.TextContent, &d.Extraction, &d.CreatedAt)
	return d, notFound(err)
}

// SetDocumentText stores extracted text.
func (s *Store) SetDocumentText(ctx context.Context, id int64, text, method string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE documents SET text_content=$2, extraction=$3 WHERE id=$1`, id, text, method)
	return err
}

// ---------------------------------------------------------------- screening queue

// ClaimNextForScreening atomically takes one pending application.
// SKIP LOCKED lets several workers run without picking the same row.
func (s *Store) ClaimNextForScreening(ctx context.Context) (int64, error) {
	var id int64
	err := s.DB.QueryRowContext(ctx, `UPDATE applications SET screening_status='processing', screening_started_at=now(),
		screening_attempts=screening_attempts+1, updated_at=now()
		WHERE id = (SELECT id FROM applications WHERE screening_status='pending' ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1)
		RETURNING id`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}

// SaveScreening stores the AI results.
func (s *Store) SaveScreening(ctx context.Context, appID int64, r scoring.Result, p profile.Profile, flags []profile.Flag, model string) error {
	if flags == nil {
		flags = []profile.Flag{}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var jobID int64
	if err := tx.QueryRowContext(ctx, `UPDATE applications SET screening_status='done', screening_error='',
		score_total=$2, score_experience=$3, score_education=$4, score_duties=$5, score_cover=$6,
		scoring=$7, profile=$8, flags=$9, scored_at=now(), model=$10, prompt_version=$11, updated_at=now()
		WHERE id=$1 RETURNING job_id`,
		appID, r.Total, r.Experience, r.Education, r.Duties, r.CoverLetter, toJSON(r), toJSON(p), toJSON(flags),
		model, scoring.PromptVersion).Scan(&jobID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO events(application_id, job_id, kind, detail) VALUES ($1,$2,'screened',$3)`,
		appID, jobID, toJSON(map[string]any{"total": r.Total, "band": r.Band, "model": model})); err != nil {
		return err
	}
	return tx.Commit()
}

// FailScreening records a failure. retry puts it back in the queue.
func (s *Store) FailScreening(ctx context.Context, appID int64, msg string, status string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE applications SET screening_status=$2, screening_error=$3, updated_at=now() WHERE id=$1`,
		appID, status, msg)
	if err == nil && status != "pending" {
		_ = s.AddEvent(ctx, appID, 0, nil, "screening_failed", map[string]any{"error": msg, "status": status})
	}
	return err
}

// RequeueStale returns stuck "processing" rows (e.g. after a restart) to the queue.
func (s *Store) RequeueStale(ctx context.Context, olderThan time.Duration) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `UPDATE applications SET screening_status='pending'
		WHERE screening_status='processing' AND screening_started_at < $1`, time.Now().Add(-olderThan))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Requeue asks for an application to be screened again.
func (s *Store) Requeue(ctx context.Context, appID int64, actor int64) error {
	if _, err := s.DB.ExecContext(ctx, `UPDATE applications SET screening_status='pending', screening_error='', screening_attempts=0 WHERE id=$1`, appID); err != nil {
		return err
	}
	return s.AddEvent(ctx, appID, 0, &actor, "rescreen_requested", nil)
}

// RequeueJob re-screens every application for a job (e.g. after editing requirements).
func (s *Store) RequeueJob(ctx context.Context, jobID int64, actor int64) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `UPDATE applications SET screening_status='pending', screening_error='', screening_attempts=0
		WHERE job_id=$1 AND screening_status <> 'processing'`, jobID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	_ = s.AddEvent(ctx, 0, jobID, &actor, "job_rescreen_requested", map[string]any{"count": n})
	return n, nil
}

// FillCandidateFromProfile completes blank candidate fields with what the AI read from the CV.
func (s *Store) FillCandidateFromProfile(ctx context.Context, candID int64, p profile.Profile) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE candidates SET
		-- Fill a blank name, or upgrade a partial one taken from a file name ("Wanjiku" -> "Wanjiku Mwangi").
		name = CASE WHEN $2 <> '' AND (name = '' OR (length($2) > length(name) AND position(lower(name) in lower($2)) > 0)) THEN $2 ELSE name END,
		phone = CASE WHEN phone='' THEN $3 ELSE phone END,
		location = CASE WHEN location='' THEN $4 ELSE location END,
		linkedin_url = CASE WHEN linkedin_url='' THEN $5 ELSE linkedin_url END,
		updated_at = now() WHERE id=$1`, candID, p.Name, p.Phone, p.Location, p.LinkedInURL)
	if err != nil || p.Email == "" {
		return err
	}
	// Only set the email if the candidate had none and no other candidate owns it.
	_, err = s.DB.ExecContext(ctx, `UPDATE candidates SET email=$2 WHERE id=$1 AND email IS NULL
		AND NOT EXISTS (SELECT 1 FROM candidates WHERE email=$2)`, candID, p.Email)
	return err
}

// ---------------------------------------------------------------- integrations

// SaveIntegration stores or replaces a connected account.
func (s *Store) SaveIntegration(ctx context.Context, name, email, cipher string, by int64) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO integrations(name, account_email, secret_cipher, connected_by) VALUES ($1,$2,$3,$4)
		ON CONFLICT (name) DO UPDATE SET account_email=EXCLUDED.account_email, secret_cipher=EXCLUDED.secret_cipher,
		connected_by=EXCLUDED.connected_by, connected_at=now()`, name, email, cipher, by)
	return err
}

// GetIntegration loads a connected account.
func (s *Store) GetIntegration(ctx context.Context, name string) (Integration, error) {
	var i Integration
	err := s.DB.QueryRowContext(ctx, `SELECT name, account_email, secret_cipher, connected_at FROM integrations WHERE name=$1`, name).
		Scan(&i.Name, &i.AccountEmail, &i.SecretCipher, &i.ConnectedAt)
	return i, notFound(err)
}

// DeleteIntegration disconnects an account.
func (s *Store) DeleteIntegration(ctx context.Context, name string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM integrations WHERE name=$1`, name)
	return err
}
