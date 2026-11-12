// Package testdb gives integration tests a fresh, migrated Postgres database.
// Tests using it are skipped unless TEST_DATABASE_URL is set, e.g.
//
//	TEST_DATABASE_URL=postgres://ats:ats@localhost:5432/ats_test?sslmode=disable go test ./...
package testdb

import (
	"context"
	"database/sql"
	"os"
	"sync"
	"testing"

	"github.com/fmuoria/CV-Review-agent/internal/db"
)

var mu sync.Mutex

// Open resets the test database and returns a connection. Tests sharing the
// database are serialised by a package-level lock.
func Open(t *testing.T) *sql.DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	mu.Lock()
	t.Cleanup(mu.Unlock)
	ctx := context.Background()
	conn, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if _, err := conn.ExecContext(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(ctx, conn); err != nil {
		t.Fatal(err)
	}
	return conn
}
