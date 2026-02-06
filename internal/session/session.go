package session

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"

	"github.com/servicenow/claude-terminal-mid-service/internal/config"
	"github.com/servicenow/claude-terminal-mid-service/internal/crypto"
	"github.com/servicenow/claude-terminal-mid-service/internal/store"
)

// Maximum command length in bytes.
const maxCommandLength = 16384

// Minimum interval between commands per session.
const commandRateInterval = 100 * time.Millisecond

// validIDPattern matches alphanumeric strings, hyphens, and underscores only.
var validIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// OutputChunk represents a chunk of terminal output
type OutputChunk struct {
	Timestamp string `json:"timestamp"`
	Data      string `json:"data"`
}

// Credentials holds user credentials
type Credentials struct {
	AnthropicAPIKey string `json:"anthropicApiKey"`
	GitHubToken     string `json:"githubToken,omitempty"`
}

// EncryptedCredentials holds encrypted credential values.
type EncryptedCredentials struct {
	AnthropicAPIKey string `json:"anthropicApiKey"`
	GitHubToken     string `json:"githubToken,omitempty"`
}

// Session represents a Claude Code CLI session
type Session struct {
	SessionID            string
	UserID               string
	WorkspacePath        string
	EncryptedCredentials EncryptedCredentials
	Status               string
	PTY                  *os.File
	Cmd                  *exec.Cmd
	OutputBuffer         []OutputChunk
	LastActivity         time.Time
	Created              time.Time
	lastCommandTime      time.Time
	mu                   sync.RWMutex
	done                 chan struct{}
	encryptionKey        string
	outputBufferSize     int
	dbStore              *store.PostgresStore // nil when running in-memory only
}

// Manager manages all active sessions
type Manager struct {
	sessions map[string]*Session
	config   *config.Config
	store    *store.PostgresStore // nil when running in-memory only
	mu       sync.RWMutex
}

// NewManager creates a new session manager.
// The store parameter is optional; pass nil to use in-memory only.
func NewManager(cfg *config.Config, pgStore *store.PostgresStore) *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
		config:   cfg,
		store:    pgStore,
	}
}

// CreateSession creates a new Claude Code CLI session
func (m *Manager) CreateSession(userID string, credentials Credentials, workspaceType string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// C4: Validate userID is alphanumeric/hyphens/underscores only
	if !validIDPattern.MatchString(userID) {
		return nil, fmt.Errorf("invalid userID: must be alphanumeric, hyphens, or underscores")
	}

	// Check user session limit
	userSessions := m.getUserSessions(userID)
	activeSessions := 0
	for _, s := range userSessions {
		if s.Status == "active" || s.Status == "initializing" {
			activeSessions++
		}
	}

	if activeSessions >= m.config.Session.MaxPerUser {
		return nil, fmt.Errorf("maximum sessions per user (%d) reached", m.config.Session.MaxPerUser)
	}

	// Create session
	sessionID := uuid.New().String()
	workspacePath := filepath.Join(m.config.Workspace.BasePath, userID, sessionID)

	// C4: Verify workspace path resolves under BasePath
	absWorkspace, err := filepath.Abs(workspacePath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve workspace path: %w", err)
	}
	absBase, err := filepath.Abs(m.config.Workspace.BasePath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve base path: %w", err)
	}
	if !strings.HasPrefix(absWorkspace, absBase+string(filepath.Separator)) {
		return nil, fmt.Errorf("workspace path traversal detected")
	}

	// C6: Encrypt credentials at rest
	encCreds := EncryptedCredentials{}
	encKey := m.config.Security.EncryptionKey
	if encKey != "" {
		encAPI, err := crypto.Encrypt([]byte(credentials.AnthropicAPIKey), encKey)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt API key: %w", err)
		}
		encCreds.AnthropicAPIKey = encAPI

		if credentials.GitHubToken != "" {
			encGH, err := crypto.Encrypt([]byte(credentials.GitHubToken), encKey)
			if err != nil {
				return nil, fmt.Errorf("failed to encrypt GitHub token: %w", err)
			}
			encCreds.GitHubToken = encGH
		}
	} else {
		// No encryption key configured; store raw (log a warning)
		log.Warn("ENCRYPTION_KEY not configured; credentials stored unencrypted")
		encCreds.AnthropicAPIKey = credentials.AnthropicAPIKey
		encCreds.GitHubToken = credentials.GitHubToken
	}

	session := &Session{
		SessionID:            sessionID,
		UserID:               userID,
		WorkspacePath:        absWorkspace,
		EncryptedCredentials: encCreds,
		Status:               "initializing",
		OutputBuffer:         make([]OutputChunk, 0),
		LastActivity:         time.Now(),
		Created:              time.Now(),
		done:                 make(chan struct{}),
		encryptionKey:        encKey,
		outputBufferSize:     m.config.Session.OutputBufferSize,
		dbStore:              m.store,
	}

	// Initialize session - pass raw credentials for env setup
	if err := session.Initialize(credentials); err != nil {
		return nil, fmt.Errorf("failed to initialize session: %w", err)
	}

	m.sessions[sessionID] = session

	// Persist to PostgreSQL (async, non-blocking).
	if m.store != nil {
		go m.saveSessionToDB(session)
	}

	log.WithFields(log.Fields{
		"session_id": sessionID,
		"user_id":    userID,
	}).Info("Session created successfully")

	return session, nil
}

// GetSession retrieves a session by ID, optionally verifying ownership.
func (m *Manager) GetSession(sessionID string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session not found")
	}

	return session, nil
}

// GetSessionForUser retrieves a session by ID and verifies the requesting user owns it (H1).
func (m *Manager) GetSessionForUser(sessionID, userID string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session not found")
	}

	if session.UserID != userID {
		return nil, fmt.Errorf("session not found")
	}

	return session, nil
}

// ListSessionsForUser returns a summary of all sessions owned by the given user (H10).
func (m *Manager) ListSessionsForUser(userID string) []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.getUserSessions(userID)
}

// TerminateSession terminates and cleans up a session
func (m *Manager) TerminateSession(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found")
	}

	if err := session.Cleanup(); err != nil {
		log.WithFields(log.Fields{
			"session_id": sessionID,
			"error":      err,
		}).Error("Error cleaning up session")
	}

	delete(m.sessions, sessionID)

	// Update status in DB then delete the record.
	if m.store != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := m.store.UpdateSessionStatus(ctx, sessionID, "terminated"); err != nil {
				log.WithError(err).WithField("session_id", sessionID).Warn("Failed to update session status in DB")
			}
			if err := m.store.DeleteSession(ctx, sessionID); err != nil {
				log.WithError(err).WithField("session_id", sessionID).Warn("Failed to delete session from DB")
			}
		}()
	}

	log.WithFields(log.Fields{
		"session_id": sessionID,
	}).Info("Session terminated")

	return nil
}

// TerminateSessionForUser terminates a session after verifying ownership (H1).
func (m *Manager) TerminateSessionForUser(sessionID, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found")
	}

	if session.UserID != userID {
		return fmt.Errorf("session not found")
	}

	if err := session.Cleanup(); err != nil {
		log.WithFields(log.Fields{
			"session_id": sessionID,
			"error":      err,
		}).Error("Error cleaning up session")
	}

	delete(m.sessions, sessionID)

	// Update status in DB then delete the record.
	if m.store != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := m.store.UpdateSessionStatus(ctx, sessionID, "terminated"); err != nil {
				log.WithError(err).WithField("session_id", sessionID).Warn("Failed to update session status in DB")
			}
			if err := m.store.DeleteSession(ctx, sessionID); err != nil {
				log.WithError(err).WithField("session_id", sessionID).Warn("Failed to delete session from DB")
			}
		}()
	}

	log.WithFields(log.Fields{
		"session_id": sessionID,
	}).Info("Session terminated")

	return nil
}

// getUserSessions returns all sessions for a specific user (must be called with lock held)
func (m *Manager) getUserSessions(userID string) []*Session {
	var userSessions []*Session
	for _, session := range m.sessions {
		if session.UserID == userID {
			userSessions = append(userSessions, session)
		}
	}
	return userSessions
}

// CleanupAll cleans up all sessions
func (m *Manager) CleanupAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for sessionID, session := range m.sessions {
		if err := session.Cleanup(); err != nil {
			log.WithFields(log.Fields{
				"session_id": sessionID,
				"error":      err,
			}).Error("Error cleaning up session")
		}
	}

	m.sessions = make(map[string]*Session)
	log.Info("All sessions cleaned up")
}

// ActiveSessionCount returns the number of sessions currently tracked.
func (m *Manager) ActiveSessionCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

// StartTimeoutChecker starts a goroutine that checks for timed out sessions
func (m *Manager) StartTimeoutChecker(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	log.Info("Session timeout checker started")

	for {
		select {
		case <-ctx.Done():
			log.Info("Session timeout checker stopped")
			return
		case <-ticker.C:
			m.checkTimeouts()
		}
	}
}

func (m *Manager) checkTimeouts() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	timeoutDuration := time.Duration(m.config.Session.TimeoutMinutes) * time.Minute

	for sessionID, session := range m.sessions {
		if session.Status == "active" && now.Sub(session.LastActivity) > timeoutDuration {
			log.WithFields(log.Fields{
				"session_id": sessionID,
				"user_id":    session.UserID,
				"idle_time":  now.Sub(session.LastActivity),
			}).Info("Session timed out")

			if err := session.Cleanup(); err != nil {
				log.WithFields(log.Fields{
					"session_id": sessionID,
					"error":      err,
				}).Error("Error cleaning up timed out session")
			}

			delete(m.sessions, sessionID)

			// Update DB status for timed-out session.
			if m.store != nil {
				sid := sessionID
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					if err := m.store.UpdateSessionStatus(ctx, sid, "terminated"); err != nil {
						log.WithError(err).WithField("session_id", sid).Warn("Failed to update timed-out session status in DB")
					}
				}()
			}
		}
	}
}

// Initialize initializes the Claude Code CLI session.
// Raw credentials are passed only for environment setup and are not retained.
func (s *Session) Initialize(creds Credentials) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	log.WithFields(log.Fields{
		"session_id": s.SessionID,
		"user_id":    s.UserID,
	}).Info("Initializing session")

	// Create workspace directory
	if err := os.MkdirAll(s.WorkspacePath, 0755); err != nil {
		return fmt.Errorf("failed to create workspace: %w", err)
	}

	// Set up command
	cmd := exec.Command("claude", "code")
	cmd.Dir = s.WorkspacePath

	// Set up environment using raw credentials (not stored)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("ANTHROPIC_API_KEY=%s", creds.AnthropicAPIKey),
		fmt.Sprintf("HOME=%s", s.WorkspacePath),
		fmt.Sprintf("PWD=%s", s.WorkspacePath),
	)

	if creds.GitHubToken != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("GITHUB_TOKEN=%s", creds.GitHubToken))
	}

	// Start the command with a PTY
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return fmt.Errorf("failed to start PTY: %w", err)
	}

	s.PTY = ptmx
	s.Cmd = cmd
	s.Status = "active"

	// H2: Start output reader with done channel for clean exit
	go s.readOutput()

	log.WithFields(log.Fields{
		"session_id": s.SessionID,
	}).Info("Session initialized successfully")

	return nil
}

// readOutput reads output from the PTY. H2: exits when done channel is closed.
func (s *Session) readOutput() {
	buffer := make([]byte, 4096)

	for {
		select {
		case <-s.done:
			return
		default:
		}

		n, err := s.PTY.Read(buffer)
		if err != nil {
			if err != io.EOF {
				log.WithFields(log.Fields{
					"session_id": s.SessionID,
					"error":      err,
				}).Error("Error reading from PTY")
			}
			break
		}

		if n > 0 {
			s.handleOutput(string(buffer[:n]))
		}
	}

	s.mu.Lock()
	s.Status = "terminated"
	s.mu.Unlock()

	log.WithFields(log.Fields{
		"session_id": s.SessionID,
	}).Info("Output reader terminated")
}

// handleOutput processes output from the PTY
func (s *Session) handleOutput(data string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	s.LastActivity = now

	// Add to output buffer
	chunk := OutputChunk{
		Timestamp: now.Format(time.RFC3339),
		Data:      data,
	}

	s.OutputBuffer = append(s.OutputBuffer, chunk)

	// H8: Use configurable buffer size instead of hardcoded 100
	maxSize := s.outputBufferSize
	if maxSize <= 0 {
		maxSize = 100
	}
	if len(s.OutputBuffer) > maxSize {
		s.OutputBuffer = s.OutputBuffer[len(s.OutputBuffer)-maxSize:]
	}

	// Persist output chunk to DB (async, never block PTY).
	if s.dbStore != nil {
		sid := s.SessionID
		ts := now
		d := data
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := s.dbStore.SaveOutputChunk(ctx, sid, ts, d); err != nil {
				log.WithError(err).WithField("session_id", sid).Warn("Failed to save output chunk to DB")
			}
		}()
	}
}

// sanitizeCommand filters dangerous control characters from input (C3).
func sanitizeCommand(command string) string {
	var b strings.Builder
	b.Grow(len(command))
	for _, r := range command {
		// Allow printable characters, newline (0x0A), carriage return (0x0D), tab (0x09)
		if r == 0x0A || r == 0x0D || r == 0x09 || r >= 0x20 {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// SendCommand sends a command to the Claude Code CLI (C3: with sanitization & rate limiting)
func (s *Session) SendCommand(command string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Status != "active" {
		return fmt.Errorf("session is not active (status: %s)", s.Status)
	}

	// C3: Command length limit
	if len(command) > maxCommandLength {
		return fmt.Errorf("command too long (max %d bytes)", maxCommandLength)
	}

	// C3: Rate limiting per session
	now := time.Now()
	if now.Sub(s.lastCommandTime) < commandRateInterval {
		return fmt.Errorf("command rate limit exceeded, try again shortly")
	}
	s.lastCommandTime = now

	// C3: Sanitize control characters
	command = sanitizeCommand(command)

	s.LastActivity = now

	_, err := s.PTY.Write([]byte(command))
	if err != nil {
		return fmt.Errorf("failed to write command: %w", err)
	}

	// Persist last activity to DB (async, never block command path).
	if s.dbStore != nil {
		sid := s.SessionID
		ts := now
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := s.dbStore.UpdateLastActivity(ctx, sid, ts); err != nil {
				log.WithError(err).WithField("session_id", sid).Warn("Failed to update last activity in DB")
			}
		}()
	}

	log.WithFields(log.Fields{
		"session_id": s.SessionID,
		"command":    command[:min(50, len(command))],
	}).Debug("Command sent to session")

	return nil
}

// GetOutput retrieves the output buffer
func (s *Session) GetOutput(clear bool) []OutputChunk {
	s.mu.Lock()
	defer s.mu.Unlock()

	output := make([]OutputChunk, len(s.OutputBuffer))
	copy(output, s.OutputBuffer)

	if clear {
		s.OutputBuffer = make([]OutputChunk, 0)
	}

	return output
}

// Resize resizes the PTY
func (s *Session) Resize(cols, rows int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.PTY == nil {
		return fmt.Errorf("PTY not initialized")
	}

	if err := pty.Setsize(s.PTY, &pty.Winsize{
		Rows: uint16(rows),
		Cols: uint16(cols),
	}); err != nil {
		return fmt.Errorf("failed to resize PTY: %w", err)
	}

	return nil
}

// GetStatus returns the current session status
func (s *Session) GetStatus() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]interface{}{
		"session_id":         s.SessionID,
		"user_id":            s.UserID,
		"status":             s.Status,
		"workspace_path":     s.WorkspacePath,
		"last_activity":      s.LastActivity.Format(time.RFC3339),
		"created":            s.Created.Format(time.RFC3339),
		"output_buffer_size": len(s.OutputBuffer),
	}
}

// Cleanup cleans up the session resources. H2: signals done channel to stop readOutput.
func (s *Session) Cleanup() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	log.WithFields(log.Fields{
		"session_id": s.SessionID,
	}).Info("Cleaning up session")

	// H2: Signal readOutput goroutine to stop
	select {
	case <-s.done:
		// Already closed
	default:
		close(s.done)
	}

	// Kill the process
	if s.Cmd != nil && s.Cmd.Process != nil {
		if err := s.Cmd.Process.Kill(); err != nil {
			log.WithFields(log.Fields{
				"session_id": s.SessionID,
				"error":      err,
			}).Warn("Error killing process")
		}
	}

	// Close PTY
	if s.PTY != nil {
		if err := s.PTY.Close(); err != nil {
			log.WithFields(log.Fields{
				"session_id": s.SessionID,
				"error":      err,
			}).Warn("Error closing PTY")
		}
	}

	// Clean up workspace (if isolated type)
	if err := os.RemoveAll(s.WorkspacePath); err != nil {
		log.WithFields(log.Fields{
			"session_id": s.SessionID,
			"error":      err,
		}).Warn("Error removing workspace")
	}

	s.Status = "terminated"

	return nil
}

// MarshalJSON implements json.Marshaler
func (s *Session) MarshalJSON() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return json.Marshal(map[string]interface{}{
		"session_id":     s.SessionID,
		"user_id":        s.UserID,
		"workspace_path": s.WorkspacePath,
		"status":         s.Status,
		"last_activity":  s.LastActivity.Format(time.RFC3339),
		"created":        s.Created.Format(time.RFC3339),
	})
}

// saveSessionToDB persists a session record to PostgreSQL.
// Called asynchronously; errors are logged, never returned to callers.
func (m *Manager) saveSessionToDB(s *Session) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	s.mu.RLock()
	credsJSON, err := json.Marshal(s.EncryptedCredentials)
	if err != nil {
		log.WithError(err).WithField("session_id", s.SessionID).Warn("Failed to marshal encrypted credentials for DB")
		s.mu.RUnlock()
		return
	}
	rec := store.SessionRecord{
		SessionID:            s.SessionID,
		UserID:               s.UserID,
		WorkspacePath:        s.WorkspacePath,
		Status:               s.Status,
		EncryptedCredentials: credsJSON,
		LastActivity:         s.LastActivity,
		CreatedAt:            s.Created,
	}
	s.mu.RUnlock()

	if err := m.store.SaveSession(ctx, rec); err != nil {
		log.WithError(err).WithField("session_id", rec.SessionID).Warn("Failed to save session to DB")
	}
}

// RecoverSessions marks stale sessions as terminated on startup.
// This should be called once during initialization.
func (m *Manager) RecoverSessions(ctx context.Context) {
	if m.store == nil {
		return
	}

	count, err := m.store.MarkStaleSessionsTerminated(ctx)
	if err != nil {
		log.WithError(err).Error("Failed to mark stale sessions as terminated")
		return
	}

	if count > 0 {
		log.WithField("count", count).Info("Marked stale sessions as terminated on startup")
	} else {
		log.Info("No stale sessions found on startup")
	}
}
