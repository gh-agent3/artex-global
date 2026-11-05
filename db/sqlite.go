package db

// SQLite backend (experimental option) — a single-user/local deployment can
// point ARTEX_PG_DSN (or database.dsn) at a modernc.org/sqlite URI such as
// "file:/path/artex.db?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)".
// PostgreSQL stays the default and the only fully validated backend; this file
// makes the core paths work on SQLite by (a) applying the translated schema
// (schema_sqlite.sql) and (b) rewriting the small set of PG-isms those paths
// hit, funnelled through the shadowed Query/Exec methods below.
//
// Scope note (docs/backlog.md item 6): the dialect translation covers the core
// single-user paths exercised by TestSQLiteCore in this package. PG-only SQL
// that the rewrite does not cover fails loudly at the query site, not silently.

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"regexp"
	"strings"
)

//go:embed schema_sqlite.sql
var schemaSQLiteSQL string

// isSQLiteDSN distinguishes backends by DSN scheme: modernc.org/sqlite uses
// file: URIs. Anything else goes down the PostgreSQL path.
func isSQLiteDSN(dsn string) bool {
	s := strings.TrimSpace(dsn)
	return strings.HasPrefix(strings.ToLower(s), "file:")
}

// openSQLite opens a SQLite database, applies the translated schema idempotently
// (every startup replays it; CREATE TABLE IF NOT EXISTS / guarded ALTERs make
// that safe), and returns a DB in sqlite dialect mode.
func openSQLite(dsn string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	// One writer at a time is SQLite's rule; a small pool of readers is fine
	// under WAL. busy_timeout (set via _pragma in the DSN) turns lock
	// contention into waiting instead of SQLITE_BUSY errors.
	sqlDB.SetMaxOpenConns(8)
	sqlDB.SetMaxIdleConns(4)
	d := &DB{DB: sqlDB, sqlite: true}

	conn, err := sqlDB.Conn(context.Background())
	if err != nil {
		sqlDB.Close()
		return nil, err
	}
	defer conn.Close()
	for i, stmt := range sqliteStatements(schemaSQLiteSQL) {
		if _, err := conn.ExecContext(context.Background(), stmt); err != nil {
			head := stmt
			if idx := strings.IndexByte(head, '\n'); idx > 0 {
				head = head[:idx]
			}
			return nil, fmt.Errorf("apply sqlite schema: statement %d: %w\n  > %s", i+1, err, head)
		}
	}
	return d, nil
}

// sqliteStatements splits the schema script into individual statements,
// respecting the CREATE TRIGGER ... BEGIN ... END; shape (an inner ";" must not
// terminate the statement early).
func sqliteStatements(script string) []string {
	var out []string
	var buf strings.Builder
	inTrigger := false
	for _, line := range strings.Split(script, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		buf.WriteString(line)
		buf.WriteString("\n")
		upper := strings.ToUpper(trimmed)
		switch {
		case !inTrigger && strings.HasPrefix(upper, "CREATE TRIGGER"):
			inTrigger = true
		case inTrigger && strings.HasPrefix(upper, "END;"):
			inTrigger = false
		}
		if !inTrigger && strings.HasSuffix(trimmed, ";") {
			if s := strings.TrimSpace(buf.String()); s != "" {
				out = append(out, s)
			}
			buf.Reset()
		}
	}
	if s := strings.TrimSpace(buf.String()); s != "" {
		out = append(out, s)
	}
	return out
}

var (
	reSQLiteCast   = regexp.MustCompile(`::[a-zA-Z_]+(\[\])?`)
	reSQLiteForUpd = regexp.MustCompile(`(?i)\s+FOR UPDATE(\s+OF\s+[a-z_ ,]+)?(\s+SKIP LOCKED|\s+NOWAIT)?`)
	reSQLiteILike  = regexp.MustCompile(`(?i)\bILIKE\b`)
	reSQLiteAdvLock = regexp.MustCompile(`(?i)^\s*SELECT\s+pg_advisory_xact_lock\([^)]*\)\s*;?\s*$`)
)

// rewriteSQLite translates the PG-isms the core paths emit into SQLite. Functions
// like now(), regexp() and greatest() are provided by sqlite_compat.go via the
// driver, so they need no textual rewrite.
func rewriteSQLite(q string) string {
	if reSQLiteAdvLock.MatchString(q) {
		return "SELECT 1"
	}
	q = reSQLiteCast.ReplaceAllString(q, "")
	q = reSQLiteForUpd.ReplaceAllString(q, "")
	q = reSQLiteILike.ReplaceAllString(q, "LIKE")
	return q
}

// The shadowed entry points funnel every query in the package through
// rewriteSQLite when the dialect is SQLite. Embedded *sql.DB methods with the
// same names would otherwise bypass the rewrite.

func (d *DB) Query(query string, args ...any) (*sql.Rows, error) {
	if d.sqlite {
		query = rewriteSQLite(query)
	}
	return d.DB.Query(query, args...)
}

func (d *DB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if d.sqlite {
		query = rewriteSQLite(query)
	}
	return d.DB.QueryContext(ctx, query, args...)
}

func (d *DB) QueryRow(query string, args ...any) *sql.Row {
	if d.sqlite {
		query = rewriteSQLite(query)
	}
	return d.DB.QueryRow(query, args...)
}

func (d *DB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	if d.sqlite {
		query = rewriteSQLite(query)
	}
	return d.DB.QueryRowContext(ctx, query, args...)
}

func (d *DB) Exec(query string, args ...any) (sql.Result, error) {
	if d.sqlite {
		query = rewriteSQLite(query)
	}
	return d.DB.Exec(query, args...)
}

func (d *DB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if d.sqlite {
		query = rewriteSQLite(query)
	}
	return d.DB.ExecContext(ctx, query, args...)
}
