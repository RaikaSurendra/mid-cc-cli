package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/servicenow/claude-terminal-mid-service/internal/config"
	"github.com/servicenow/claude-terminal-mid-service/internal/session"
)

func setupTestServer() (*Server, *gin.Engine) {
	gin.SetMode(gin.TestMode)

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
		Security: config.SecurityConfig{},
	}

	sessionManager := session.NewManager(cfg, nil)
	router := gin.New()

	srv := New(cfg, sessionManager, router)
	srv.RegisterRoutes()

	return srv, router
}

func TestHealthEndpoint(t *testing.T) {
	_, router := setupTestServer()

	req, _ := http.NewRequest("GET", "/health", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.Code)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if result["status"] != "healthy" {
		t.Errorf("Expected status healthy, got %v", result["status"])
	}

	// H6: Verify new health fields are present
	if _, ok := result["timestamp"]; !ok {
		t.Error("Expected 'timestamp' in health response")
	}
	if _, ok := result["active_sessions"]; !ok {
		t.Error("Expected 'active_sessions' in health response")
	}
	if _, ok := result["memory_alloc_mb"]; !ok {
		t.Error("Expected 'memory_alloc_mb' in health response")
	}
}

func TestCreateSessionMissingFields(t *testing.T) {
	_, router := setupTestServer()

	reqBody := []byte(`{"userId": "test"}`)
	req, _ := http.NewRequest("POST", "/api/session/create", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.Code)
	}
}

func TestCreateSessionValidRequest(t *testing.T) {
	_, router := setupTestServer()

	reqBody := []byte(`{
		"userId": "test-user",
		"credentials": {
			"anthropicApiKey": "test-key"
		},
		"workspaceType": "isolated"
	}`)

	req, _ := http.NewRequest("POST", "/api/session/create", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	// May fail without Claude CLI, but should get proper error
	if resp.Code != http.StatusOK && resp.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 200 or 500, got %d", resp.Code)
	}
}

func TestGetStatusNonExistentSession(t *testing.T) {
	_, router := setupTestServer()

	req, _ := http.NewRequest("GET", "/api/session/non-existent-id/status", nil)
	req.Header.Set("X-User-ID", "test-user")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", resp.Code)
	}
}

// Test that session endpoints require X-User-ID header
func TestSessionEndpointRequiresUserID(t *testing.T) {
	_, router := setupTestServer()

	// No X-User-ID header -> should get 400
	req, _ := http.NewRequest("GET", "/api/session/some-id/status", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 without X-User-ID, got %d", resp.Code)
	}
}

func TestSendCommandMissingBody(t *testing.T) {
	_, router := setupTestServer()

	req, _ := http.NewRequest("POST", "/api/session/test-id/command", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.Code)
	}
}

func TestResizeMissingParameters(t *testing.T) {
	_, router := setupTestServer()

	reqBody := []byte(`{"cols": 80}`) // Missing rows
	req, _ := http.NewRequest("POST", "/api/session/test-id/resize", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.Code)
	}
}

// C1: Test auth middleware blocks unauthenticated requests when token is configured
func TestAuthMiddlewareRejectsNoToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

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
		Security: config.SecurityConfig{
			APIAuthToken: "test-secret-token",
		},
	}

	sessionManager := session.NewManager(cfg, nil)
	router := gin.New()
	srv := New(cfg, sessionManager, router)
	srv.RegisterRoutes()

	req, _ := http.NewRequest("GET", "/api/sessions", nil)
	req.Header.Set("X-User-ID", "test")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401 without token, got %d", resp.Code)
	}
}

// C1: Test auth middleware allows valid token
func TestAuthMiddlewareAcceptsValidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

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
		Security: config.SecurityConfig{
			APIAuthToken: "test-secret-token",
		},
	}

	sessionManager := session.NewManager(cfg, nil)
	router := gin.New()
	srv := New(cfg, sessionManager, router)
	srv.RegisterRoutes()

	req, _ := http.NewRequest("GET", "/api/sessions", nil)
	req.Header.Set("Authorization", "Bearer test-secret-token")
	req.Header.Set("X-User-ID", "test")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code == http.StatusUnauthorized {
		t.Errorf("Expected auth to succeed with valid token, got 401")
	}
}

// Benchmark tests

func BenchmarkHealthEndpoint(b *testing.B) {
	_, router := setupTestServer()

	req, _ := http.NewRequest("GET", "/health", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
	}
}

func BenchmarkCreateSessionRequest(b *testing.B) {
	_, router := setupTestServer()

	reqBody := []byte(`{
		"userId": "bench-user",
		"credentials": {
			"anthropicApiKey": "bench-key"
		}
	}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("POST", "/api/session/create", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
	}
}

func BenchmarkGetStatusRequest(b *testing.B) {
	_, router := setupTestServer()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("GET", "/api/session/test-id/status", nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
	}
}
