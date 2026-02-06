package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	log "github.com/sirupsen/logrus"

	"github.com/servicenow/claude-terminal-mid-service/internal/config"
)

// SessionRecord represents a session row stored in PostgreSQL.
type SessionRecord struct {
	SessionID            string          `json:"session_id"`
	UserID               string          `json:"user_id"`
	WorkspacePath        string          `json:"workspace_path"`
	Status               string          `json:"status"`
	EncryptedCredentials json.RawMessage `json:"encrypted_credentials,omitempty"`
	LastActivity         time.Time       `json:"last_activity"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

// OutputChunk represents a row in the session_output table.
type OutputChunk struct {
	ID        int64     `json:"id"`
	SessionID string    `json:"session_id"`
	Timestamp time.Time `json:"timestamp"`
	Data      string    `json:"data"`
}

// PostgresStore implements persistent session storage backed by PostgreSQL.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// migration DDL executed on startup.
const migrationSQL = `
CREATE TABLE IF NOT EXISTS sessions (
    session_id VARCHAR(36) PRIMARY KEY,
    user_id VARCHAR(255) NOT NULL,
    workspace_path TEXT NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'initializing',
    encrypted_credentials JSONB,
    last_activity TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(status);

CREATE TABLE IF NOT EXISTS session_output (
    id BIGSERIAL PRIMARY KEY,
    session_id VARCHAR(36) NOT NULL REFERENCES sessions(session_id) ON DELETE CASCADE,
    timestamp TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    data TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_session_output_session_id ON session_output(session_id);
`

// NewPostgresStore creates a connection pool and runs migrations.
func NewPostgresStore(ctx context.Context, dbCfg config.DatabaseConfig) (*PostgresStore, error) {
	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		dbCfg.User, dbCfg.Password, dbCfg.Host, dbCfg.Port, dbCfg.DBName, dbCfg.SSLMode,
	)

	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database config: %w", err)
	}

	// Sensible pool defaults.
	poolCfg.MaxConns = 10
	poolCfg.MinConns = 2
	poolCfg.MaxConnLifetime = 30 * time.Minute
	poolCfg.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Verify connectivity.
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Info("PostgreSQL connection pool established")

	// Run migration.
	if _, err := pool.Exec(ctx, migrationSQL); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to run database migration: %w", err)
	}

	log.Info("PostgreSQL migration completed")

	return &PostgresStore{pool: pool}, nil
}

// SaveSession inserts or updates (upserts) a session record.
func (s *PostgresStore) SaveSession(ctx context.Context, rec SessionRecord) error {
	query := `
		INSERT INTO sessions (session_id, user_id, workspace_path, status, encrypted_credentials, last_activity, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		ON CONFLICT (session_id) DO UPDATE SET
			user_id = EXCLUDED.user_id,
			workspace_path = EXCLUDED.workspace_path,
			status = EXCLUDED.status,
			encrypted_credentials = EXCLUDED.encrypted_credentials,
			last_activity = EXCLUDED.last_activity,
			updated_at = NOW()
	`
	_, err := s.pool.Exec(ctx, query,
		rec.SessionID,
		rec.UserID,
		rec.WorkspacePath,
		rec.Status,
		rec.EncryptedCredentials,
		rec.LastActivity,
		rec.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("SaveSession: %w", err)
	}
	return nil
}

// GetSession retrieves a single session by ID.
func (s *PostgresStore) GetSession(ctx context.Context, sessionID string) (*SessionRecord, error) {
	query := `
		SELECT session_id, user_id, workspace_path, status, encrypted_credentials, last_activity, created_at, updated_at
		FROM sessions
		WHERE session_id = $1
	`
	row := s.pool.QueryRow(ctx, query, sessionID)

	var rec SessionRecord
	if err := row.Scan(
		&rec.SessionID,
		&rec.UserID,
		&rec.WorkspacePath,
		&rec.Status,
		&rec.EncryptedCredentials,
		&rec.LastActivity,
		&rec.CreatedAt,
		&rec.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("GetSession: %w", err)
	}
	return &rec, nil
}

// GetSessionsForUser returns all sessions belonging to a user.
func (s *PostgresStore) GetSessionsForUser(ctx context.Context, userID string) ([]SessionRecord, error) {
	query := `
		SELECT session_id, user_id, workspace_path, status, encrypted_credentials, last_activity, created_at, updated_at
		FROM sessions
		WHERE user_id = $1
		ORDER BY created_at DESC
	`
	rows, err := s.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("GetSessionsForUser: %w", err)
	}
	defer rows.Close()

	var records []SessionRecord
	for rows.Next() {
		var rec SessionRecord
		if err := rows.Scan(
			&rec.SessionID,
			&rec.UserID,
			&rec.WorkspacePath,
			&rec.Status,
			&rec.EncryptedCredentials,
			&rec.LastActivity,
			&rec.CreatedAt,
			&rec.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("GetSessionsForUser scan: %w", err)
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

// UpdateSessionStatus sets the status column for a session.
func (s *PostgresStore) UpdateSessionStatus(ctx context.Context, sessionID, status string) error {
	query := `UPDATE sessions SET status = $1, updated_at = NOW() WHERE session_id = $2`
	_, err := s.pool.Exec(ctx, query, status, sessionID)
	if err != nil {
		return fmt.Errorf("UpdateSessionStatus: %w", err)
	}
	return nil
}

// UpdateLastActivity bumps the last_activity timestamp.
func (s *PostgresStore) UpdateLastActivity(ctx context.Context, sessionID string, t time.Time) error {
	query := `UPDATE sessions SET last_activity = $1, updated_at = NOW() WHERE session_id = $2`
	_, err := s.pool.Exec(ctx, query, t, sessionID)
	if err != nil {
		return fmt.Errorf("UpdateLastActivity: %w", err)
	}
	return nil
}

// SaveOutputChunk appends a terminal output chunk for a session.
func (s *PostgresStore) SaveOutputChunk(ctx context.Context, sessionID string, timestamp time.Time, data string) error {
	query := `INSERT INTO session_output (session_id, timestamp, data) VALUES ($1, $2, $3)`
	_, err := s.pool.Exec(ctx, query, sessionID, timestamp, data)
	if err != nil {
		return fmt.Errorf("SaveOutputChunk: %w", err)
	}
	return nil
}

// GetOutputChunks returns the most recent output chunks for a session.
func (s *PostgresStore) GetOutputChunks(ctx context.Context, sessionID string, limit int) ([]OutputChunk, error) {
	query := `
		SELECT id, session_id, timestamp, data
		FROM session_output
		WHERE session_id = $1
		ORDER BY id DESC
		LIMIT $2
	`
	rows, err := s.pool.Query(ctx, query, sessionID, limit)
	if err != nil {
		return nil, fmt.Errorf("GetOutputChunks: %w", err)
	}
	defer rows.Close()

	var chunks []OutputChunk
	for rows.Next() {
		var c OutputChunk
		if err := rows.Scan(&c.ID, &c.SessionID, &c.Timestamp, &c.Data); err != nil {
			return nil, fmt.Errorf("GetOutputChunks scan: %w", err)
		}
		chunks = append(chunks, c)
	}

	// Reverse so oldest is first.
	for i, j := 0, len(chunks)-1; i < j; i, j = i+1, j-1 {
		chunks[i], chunks[j] = chunks[j], chunks[i]
	}

	return chunks, rows.Err()
}

// DeleteSession removes a session and its output (cascade).
func (s *PostgresStore) DeleteSession(ctx context.Context, sessionID string) error {
	query := `DELETE FROM sessions WHERE session_id = $1`
	_, err := s.pool.Exec(ctx, query, sessionID)
	if err != nil {
		return fmt.Errorf("DeleteSession: %w", err)
	}
	return nil
}

// GetActiveSessions returns all sessions with active or initializing status.
func (s *PostgresStore) GetActiveSessions(ctx context.Context) ([]SessionRecord, error) {
	query := `
		SELECT session_id, user_id, workspace_path, status, encrypted_credentials, last_activity, created_at, updated_at
		FROM sessions
		WHERE status IN ('active', 'initializing')
		ORDER BY created_at DESC
	`
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("GetActiveSessions: %w", err)
	}
	defer rows.Close()

	var records []SessionRecord
	for rows.Next() {
		var rec SessionRecord
		if err := rows.Scan(
			&rec.SessionID,
			&rec.UserID,
			&rec.WorkspacePath,
			&rec.Status,
			&rec.EncryptedCredentials,
			&rec.LastActivity,
			&rec.CreatedAt,
			&rec.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("GetActiveSessions scan: %w", err)
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

// MarkStaleSessionsTerminated sets status='terminated' for sessions that were
// active or initializing (i.e., they had no running process after a restart).
func (s *PostgresStore) MarkStaleSessionsTerminated(ctx context.Context) (int64, error) {
	query := `
		UPDATE sessions
		SET status = 'terminated', updated_at = NOW()
		WHERE status IN ('active', 'initializing')
	`
	tag, err := s.pool.Exec(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("MarkStaleSessionsTerminated: %w", err)
	}
	return tag.RowsAffected(), nil
}

// Close closes the connection pool.
func (s *PostgresStore) Close() {
	if s.pool != nil {
		s.pool.Close()
		log.Info("PostgreSQL connection pool closed")
	}
}
