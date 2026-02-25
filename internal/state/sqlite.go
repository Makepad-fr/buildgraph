package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Makepad-fr/buildgraph/internal/backend"
	_ "modernc.org/sqlite"
)

type Store struct {
	db   *sql.DB
	path string
}

type RunRecord struct {
	Command    string
	StartedAt  time.Time
	DurationMS int64
	Success    bool
	ExitCode   int
	ErrorText  string
}

func Open(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("state db path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	store := &Store{db: db, path: path}
	if err := store.init(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) Path() string {
	return s.path
}

func (s *Store) init(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL);`,
		`CREATE TABLE IF NOT EXISTS runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			command TEXT NOT NULL,
			started_at TEXT NOT NULL,
			duration_ms INTEGER NOT NULL,
			success INTEGER NOT NULL,
			exit_code INTEGER NOT NULL,
			error_text TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS findings (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id INTEGER NOT NULL,
			rule_id TEXT NOT NULL,
			dimension TEXT NOT NULL,
			severity TEXT NOT NULL,
			message TEXT NOT NULL,
			file TEXT NOT NULL,
			line INTEGER NOT NULL,
			suggestion TEXT,
			docs_url TEXT,
			FOREIGN KEY(run_id) REFERENCES runs(id)
		);`,
		`CREATE TABLE IF NOT EXISTS builds (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id INTEGER NOT NULL,
			backend TEXT NOT NULL,
			endpoint TEXT,
			outputs_json TEXT,
			digest TEXT,
			provenance_available INTEGER NOT NULL,
			cache_hits INTEGER NOT NULL,
			cache_misses INTEGER NOT NULL,
			warnings_json TEXT,
			FOREIGN KEY(run_id) REFERENCES runs(id)
		);`,
		`CREATE TABLE IF NOT EXISTS events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id INTEGER,
			name TEXT NOT NULL,
			payload_json TEXT,
			created_at TEXT NOT NULL,
			FOREIGN KEY(run_id) REFERENCES runs(id)
		);`,
	}

	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("initialize schema: %w", err)
		}
	}

	_, _ = s.db.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(1, ?);`, time.Now().UTC().Format(time.RFC3339))
	return nil
}

func (s *Store) RecordRun(ctx context.Context, run RunRecord) (int64, error) {
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO runs(command, started_at, duration_ms, success, exit_code, error_text) VALUES(?, ?, ?, ?, ?, ?)`,
		run.Command,
		run.StartedAt.UTC().Format(time.RFC3339),
		run.DurationMS,
		boolToInt(run.Success),
		run.ExitCode,
		run.ErrorText,
	)
	if err != nil {
		return 0, fmt.Errorf("insert run: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("resolve run id: %w", err)
	}
	return id, nil
}

func (s *Store) RecordFindings(ctx context.Context, runID int64, findings []backend.Finding) error {
	if len(findings) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin finding transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO findings(run_id, rule_id, dimension, severity, message, file, line, suggestion, docs_url) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare finding insert: %w", err)
	}
	defer stmt.Close()

	for _, finding := range findings {
		if _, err := stmt.ExecContext(ctx,
			runID,
			finding.ID,
			finding.Dimension,
			finding.Severity,
			finding.Message,
			finding.File,
			finding.Line,
			finding.Suggestion,
			finding.DocsURL,
		); err != nil {
			return fmt.Errorf("insert finding: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit findings: %w", err)
	}
	return nil
}

func (s *Store) RecordBuild(ctx context.Context, runID int64, result backend.BuildResult) error {
	outputsJSON, _ := json.Marshal(result.Outputs)
	warningsJSON, _ := json.Marshal(result.Warnings)

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO builds(run_id, backend, endpoint, outputs_json, digest, provenance_available, cache_hits, cache_misses, warnings_json) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		runID,
		result.Backend,
		result.Endpoint,
		string(outputsJSON),
		result.Digest,
		boolToInt(result.ProvenanceAvailable),
		result.CacheStats.Hits,
		result.CacheStats.Misses,
		string(warningsJSON),
	)
	if err != nil {
		return fmt.Errorf("insert build: %w", err)
	}
	return nil
}

func (s *Store) RecordEvent(ctx context.Context, runID int64, name string, payload any) error {
	var payloadJSON string
	if payload != nil {
		bytes, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal event payload: %w", err)
		}
		payloadJSON = string(bytes)
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO events(run_id, name, payload_json, created_at) VALUES(?, ?, ?, ?)`,
		runID,
		name,
		payloadJSON,
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	return nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
