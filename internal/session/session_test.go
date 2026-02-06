package session

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/servicenow/claude-terminal-mid-service/internal/config"
)

func TestNewManager(t *testing.T) {
	cfg := &config.Config{
		Session: config.SessionConfig{
			TimeoutMinutes:   30,
			MaxPerUser:       3,
			OutputBufferSize: 100,
		},
		Workspace: config.WorkspaceConfig{
			BasePath: "/tmp/test-claude-sessions",
			Type:     "isolated",
		},
	}

	manager := NewManager(cfg, nil)

	if manager == nil {
		t.Fatal("NewManager returned nil")
	}

	if manager.sessions == nil {
		t.Fatal("Manager sessions map is nil")
	}

	if len(manager.sessions) != 0 {
		t.Errorf("Expected 0 sessions, got %d", len(manager.sessions))
	}
}

func TestSessionCreation(t *testing.T) {
	cfg := &config.Config{
		Session: config.SessionConfig{
			TimeoutMinutes:   30,
			MaxPerUser:       3,
			OutputBufferSize: 100,
		},
		Workspace: config.WorkspaceConfig{
			BasePath: "/tmp/test-claude-sessions",
			Type:     "isolated",
		},
	}

	manager := NewManager(cfg, nil)

	credentials := Credentials{
		AnthropicAPIKey: "test-key-12345",
		GitHubToken:     "",
	}

	// Create workspace directory
	os.MkdirAll(cfg.Workspace.BasePath, 0755)
	defer os.RemoveAll(cfg.Workspace.BasePath)

	// Note: This test will try to spawn claude CLI which may fail
	sess, err := manager.CreateSession("test-user-1", credentials, "isolated")

	if sess == nil && err != nil {
		// Expected if Claude CLI is not installed
		t.Logf("Session creation failed (expected if Claude CLI not available): %v", err)
		return
	}

	if sess != nil {
		if sess.SessionID == "" {
			t.Error("Session ID is empty")
		}

		if sess.UserID != "test-user-1" {
			t.Errorf("Expected UserID test-user-1, got %s", sess.UserID)
		}

		// Cleanup
		sess.Cleanup()
	}
}

// C4: Test invalid userID is rejected
func TestInvalidUserIDRejected(t *testing.T) {
	cfg := &config.Config{
		Session: config.SessionConfig{
			TimeoutMinutes:   30,
			MaxPerUser:       3,
			OutputBufferSize: 100,
		},
		Workspace: config.WorkspaceConfig{
			BasePath: "/tmp/test-claude-sessions",
			Type:     "isolated",
		},
	}

	manager := NewManager(cfg, nil)
	creds := Credentials{AnthropicAPIKey: "test-key"}

	// Path traversal attempt
	_, err := manager.CreateSession("../../../etc", creds, "isolated")
	if err == nil {
		t.Error("Expected error for path traversal userID, got nil")
	}

	// Control characters
	_, err = manager.CreateSession("user\x00id", creds, "isolated")
	if err == nil {
		t.Error("Expected error for userID with control characters, got nil")
	}
}

func TestSessionLimit(t *testing.T) {
	cfg := &config.Config{
		Session: config.SessionConfig{
			TimeoutMinutes:   30,
			MaxPerUser:       2, // Limit to 2
			OutputBufferSize: 100,
		},
		Workspace: config.WorkspaceConfig{
			BasePath: "/tmp/test-claude-sessions",
			Type:     "isolated",
		},
	}

	manager := NewManager(cfg, nil)

	credentials := Credentials{
		AnthropicAPIKey: "test-key-12345",
	}

	os.MkdirAll(cfg.Workspace.BasePath, 0755)
	defer os.RemoveAll(cfg.Workspace.BasePath)

	testUserID := "test-user-limit"

	// Manually add sessions to test limit
	for i := 0; i < 2; i++ {
		sess := &Session{
			SessionID:     fmt.Sprintf("session-%d", i),
			UserID:        testUserID,
			Status:        "active",
			WorkspacePath: "/tmp/test",
			Created:       time.Now(),
			LastActivity:  time.Now(),
			done:          make(chan struct{}),
		}
		manager.sessions[sess.SessionID] = sess
	}

	// Try to create third session - should fail
	_, err := manager.CreateSession(testUserID, credentials, "isolated")
	if err == nil {
		t.Error("Expected error when exceeding session limit, got nil")
	}
}

func TestSessionTimeout(t *testing.T) {
	cfg := &config.Config{
		Session: config.SessionConfig{
			TimeoutMinutes:   1, // 1 minute for testing
			MaxPerUser:       3,
			OutputBufferSize: 100,
		},
		Workspace: config.WorkspaceConfig{
			BasePath: "/tmp/test-claude-sessions",
			Type:     "isolated",
		},
	}

	manager := NewManager(cfg, nil)

	// Create old session
	oldSession := &Session{
		SessionID:     "old-session",
		UserID:        "test-user",
		Status:        "active",
		WorkspacePath: "/tmp/test",
		Created:       time.Now().Add(-2 * time.Minute),
		LastActivity:  time.Now().Add(-2 * time.Minute),
		done:          make(chan struct{}),
	}
	manager.sessions["old-session"] = oldSession

	// Create recent session
	recentSession := &Session{
		SessionID:     "recent-session",
		UserID:        "test-user",
		Status:        "active",
		WorkspacePath: "/tmp/test",
		Created:       time.Now(),
		LastActivity:  time.Now(),
		done:          make(chan struct{}),
	}
	manager.sessions["recent-session"] = recentSession

	// Check timeouts
	manager.checkTimeouts()

	// Old session should be removed
	if _, exists := manager.sessions["old-session"]; exists {
		t.Error("Old session should have been removed")
	}

	// Recent session should remain
	if _, exists := manager.sessions["recent-session"]; !exists {
		t.Error("Recent session should still exist")
	}
}

func TestOutputBuffer(t *testing.T) {
	sess := &Session{
		SessionID:        "test",
		UserID:           "test-user",
		OutputBuffer:     make([]OutputChunk, 0),
		outputBufferSize: 100,
	}

	// Add output
	sess.handleOutput("test output 1")
	sess.handleOutput("test output 2")
	sess.handleOutput("test output 3")

	if len(sess.OutputBuffer) != 3 {
		t.Errorf("Expected 3 output chunks, got %d", len(sess.OutputBuffer))
	}

	// Test get output without clear
	output := sess.GetOutput(false)
	if len(output) != 3 {
		t.Errorf("Expected 3 output chunks, got %d", len(output))
	}
	if len(sess.OutputBuffer) != 3 {
		t.Error("Buffer should not be cleared")
	}

	// Test get output with clear
	output = sess.GetOutput(true)
	if len(output) != 3 {
		t.Errorf("Expected 3 output chunks, got %d", len(output))
	}
	if len(sess.OutputBuffer) != 0 {
		t.Error("Buffer should be cleared")
	}
}

func TestOutputBufferLimit(t *testing.T) {
	sess := &Session{
		SessionID:        "test",
		UserID:           "test-user",
		OutputBuffer:     make([]OutputChunk, 0),
		outputBufferSize: 100,
	}

	// Add more than 100 outputs
	for i := 0; i < 150; i++ {
		sess.handleOutput("test output")
	}

	// Buffer should be limited to 100
	if len(sess.OutputBuffer) > 100 {
		t.Errorf("Buffer should be limited to 100, got %d", len(sess.OutputBuffer))
	}
}

// H8: Test configurable buffer size
func TestOutputBufferCustomSize(t *testing.T) {
	sess := &Session{
		SessionID:        "test",
		UserID:           "test-user",
		OutputBuffer:     make([]OutputChunk, 0),
		outputBufferSize: 50,
	}

	for i := 0; i < 100; i++ {
		sess.handleOutput("test output")
	}

	if len(sess.OutputBuffer) > 50 {
		t.Errorf("Buffer should be limited to 50, got %d", len(sess.OutputBuffer))
	}
}

func TestSessionStatus(t *testing.T) {
	sess := &Session{
		SessionID:     "test-session",
		UserID:        "test-user",
		Status:        "active",
		WorkspacePath: "/tmp/test",
		Created:       time.Now(),
		LastActivity:  time.Now(),
		OutputBuffer:  make([]OutputChunk, 5),
	}

	status := sess.GetStatus()

	if status["session_id"] != "test-session" {
		t.Error("Session ID mismatch in status")
	}

	if status["user_id"] != "test-user" {
		t.Error("User ID mismatch in status")
	}

	if status["status"] != "active" {
		t.Error("Status mismatch")
	}

	if status["output_buffer_size"] != 5 {
		t.Errorf("Expected buffer size 5, got %v", status["output_buffer_size"])
	}
}

// C3: Test command sanitization
func TestSanitizeCommand(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello world", "hello world"},
		{"line1\nline2", "line1\nline2"},                 // newline preserved
		{"with\ttab", "with\ttab"},                       // tab preserved
		{"with\r\nCRLF", "with\r\nCRLF"},                // CR+LF preserved
		{"null\x00byte", "nullbyte"},                     // null removed
		{"escape\x1bseq", "escapeseq"},                   // ESC removed
		{"bell\x07char", "bellchar"},                      // BEL removed
		{"backspace\x08here", "backspacehere"},            // BS removed
	}

	for _, tt := range tests {
		got := sanitizeCommand(tt.input)
		if got != tt.expected {
			t.Errorf("sanitizeCommand(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

// H1: Test GetSessionForUser ownership
func TestGetSessionForUser(t *testing.T) {
	cfg := &config.Config{
		Session: config.SessionConfig{
			TimeoutMinutes:   30,
			MaxPerUser:       3,
			OutputBufferSize: 100,
		},
		Workspace: config.WorkspaceConfig{
			BasePath: "/tmp/test-claude-sessions",
			Type:     "isolated",
		},
	}

	manager := NewManager(cfg, nil)

	sess := &Session{
		SessionID:     "sess-1",
		UserID:        "user-alice",
		Status:        "active",
		WorkspacePath: "/tmp/test",
		Created:       time.Now(),
		LastActivity:  time.Now(),
		done:          make(chan struct{}),
	}
	manager.sessions["sess-1"] = sess

	// Owner can access
	got, err := manager.GetSessionForUser("sess-1", "user-alice")
	if err != nil {
		t.Fatalf("Expected no error for owner, got: %v", err)
	}
	if got.SessionID != "sess-1" {
		t.Errorf("Expected session sess-1, got %s", got.SessionID)
	}

	// Non-owner cannot access
	_, err = manager.GetSessionForUser("sess-1", "user-bob")
	if err == nil {
		t.Error("Expected error for non-owner, got nil")
	}
}

// Benchmark tests

func BenchmarkSessionCreation(b *testing.B) {
	cfg := &config.Config{
		Session: config.SessionConfig{
			TimeoutMinutes:   30,
			MaxPerUser:       100,
			OutputBufferSize: 100,
		},
		Workspace: config.WorkspaceConfig{
			BasePath: "/tmp/bench-claude-sessions",
			Type:     "isolated",
		},
	}

	manager := NewManager(cfg, nil)
	credentials := Credentials{
		AnthropicAPIKey: "test-key",
	}

	os.MkdirAll(cfg.Workspace.BasePath, 0755)
	defer os.RemoveAll(cfg.Workspace.BasePath)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.CreateSession("bench-user", credentials, "isolated")
	}
}

func BenchmarkOutputBuffering(b *testing.B) {
	sess := &Session{
		SessionID:        "bench",
		UserID:           "bench-user",
		OutputBuffer:     make([]OutputChunk, 0),
		outputBufferSize: 100,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sess.handleOutput("benchmark output data")
	}
}

func BenchmarkGetOutput(b *testing.B) {
	sess := &Session{
		SessionID:        "bench",
		UserID:           "bench-user",
		OutputBuffer:     make([]OutputChunk, 100),
		outputBufferSize: 100,
	}

	for i := 0; i < 100; i++ {
		sess.OutputBuffer[i] = OutputChunk{
			Timestamp: time.Now().Format(time.RFC3339),
			Data:      "test data",
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sess.GetOutput(false)
	}
}

func BenchmarkSessionStatus(b *testing.B) {
	sess := &Session{
		SessionID:     "bench",
		UserID:        "bench-user",
		Status:        "active",
		WorkspacePath: "/tmp/bench",
		Created:       time.Now(),
		LastActivity:  time.Now(),
		OutputBuffer:  make([]OutputChunk, 50),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sess.GetStatus()
	}
}

func BenchmarkTimeoutCheck(b *testing.B) {
	cfg := &config.Config{
		Session: config.SessionConfig{
			TimeoutMinutes:   30,
			MaxPerUser:       100,
			OutputBufferSize: 100,
		},
		Workspace: config.WorkspaceConfig{
			BasePath: "/tmp/bench-claude-sessions",
			Type:     "isolated",
		},
	}

	manager := NewManager(cfg, nil)

	for i := 0; i < 100; i++ {
		sess := &Session{
			SessionID:     fmt.Sprintf("bench-%d", i),
			UserID:        "bench-user",
			Status:        "active",
			WorkspacePath: "/tmp/bench",
			Created:       time.Now(),
			LastActivity:  time.Now(),
			done:          make(chan struct{}),
		}
		manager.sessions[sess.SessionID] = sess
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.checkTimeouts()
	}
}
