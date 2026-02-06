package server

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"

	"github.com/servicenow/claude-terminal-mid-service/internal/config"
	"github.com/servicenow/claude-terminal-mid-service/internal/session"
)

// Server represents the HTTP server
type Server struct {
	config         *config.Config
	sessionManager *session.Manager
	router         *gin.Engine
}

// New creates a new HTTP server
func New(cfg *config.Config, sm *session.Manager, router *gin.Engine) *Server {
	return &Server{
		config:         cfg,
		sessionManager: sm,
		router:         router,
	}
}

// RegisterRoutes registers all HTTP routes
func (s *Server) RegisterRoutes() {
	// Health check (no auth required)
	s.router.GET("/health", s.handleHealth)

	// Session management API (C1: auth middleware applied)
	api := s.router.Group("/api")
	api.Use(s.authMiddleware())
	{
		api.POST("/session/create", s.handleCreateSession)
		api.POST("/session/:sessionId/command", s.handleSendCommand)
		api.GET("/session/:sessionId/output", s.handleGetOutput)
		api.GET("/session/:sessionId/status", s.handleGetStatus)
		api.POST("/session/:sessionId/resize", s.handleResize)
		api.DELETE("/session/:sessionId", s.handleTerminateSession)
		api.GET("/sessions", s.handleListSessions)
	}
}

// C1: authMiddleware validates the bearer token / API key on all /api routes.
func (s *Server) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := s.config.Security.APIAuthToken
		if token == "" {
			log.Warn("API_AUTH_TOKEN is not configured; authentication is disabled")
			c.Next()
			return
		}

		authHeader := c.GetHeader("Authorization")
		const prefix = "Bearer "
		if len(authHeader) <= len(prefix) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing or invalid authorization header"})
			return
		}

		provided := authHeader[len(prefix):]
		if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid authentication token"})
			return
		}

		c.Next()
	}
}

// H6: Health check endpoint with real diagnostics
func (s *Server) handleHealth(c *gin.Context) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	c.JSON(http.StatusOK, gin.H{
		"status":        "healthy",
		"timestamp":     time.Now().Format(time.RFC3339),
		"active_sessions": s.sessionManager.ActiveSessionCount(),
		"memory_alloc_mb": memStats.Alloc / 1024 / 1024,
	})
}

// CreateSessionRequest represents a session creation request
type CreateSessionRequest struct {
	UserID        string              `json:"userId" binding:"required"`
	Credentials   session.Credentials `json:"credentials" binding:"required"`
	WorkspaceType string              `json:"workspaceType"`
}

// handleCreateSession handles session creation requests
func (s *Server) handleCreateSession(c *gin.Context) {
	var req CreateSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate API key is provided
	if req.Credentials.AnthropicAPIKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "anthropicApiKey is required"})
		return
	}

	sess, err := s.sessionManager.CreateSession(req.UserID, req.Credentials, req.WorkspaceType)
	if err != nil {
		log.WithError(err).Error("Failed to create session")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"sessionId":     sess.SessionID,
		"status":        sess.Status,
		"workspacePath": sess.WorkspacePath,
	})
}

// SendCommandRequest represents a command request
type SendCommandRequest struct {
	Command string `json:"command" binding:"required"`
}

// handleSendCommand handles sending commands to a session (H1: userId ownership check)
func (s *Server) handleSendCommand(c *gin.Context) {
	sessionID := c.Param("sessionId")
	userID := c.GetHeader("X-User-ID")

	var req SendCommandRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	sess, err := s.getSessionWithAuth(sessionID, userID)
	if err != nil {
		if userID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "X-User-ID header is required"})
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	if err := sess.SendCommand(req.Command); err != nil {
		log.WithError(err).Error("Failed to send command")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
	})
}

// handleGetOutput handles retrieving session output (H1: userId ownership check)
func (s *Server) handleGetOutput(c *gin.Context) {
	sessionID := c.Param("sessionId")
	userID := c.GetHeader("X-User-ID")
	clear := c.Query("clear") == "true"

	sess, err := s.getSessionWithAuth(sessionID, userID)
	if err != nil {
		if userID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "X-User-ID header is required"})
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	output := sess.GetOutput(clear)

	c.JSON(http.StatusOK, gin.H{
		"sessionId": sessionID,
		"output":    output,
		"status":    sess.Status,
	})
}

// handleGetStatus handles retrieving session status (H1: userId ownership check)
func (s *Server) handleGetStatus(c *gin.Context) {
	sessionID := c.Param("sessionId")
	userID := c.GetHeader("X-User-ID")

	sess, err := s.getSessionWithAuth(sessionID, userID)
	if err != nil {
		if userID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "X-User-ID header is required"})
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	c.JSON(http.StatusOK, sess.GetStatus())
}

// ResizeRequest represents a terminal resize request
type ResizeRequest struct {
	Cols int `json:"cols" binding:"required"`
	Rows int `json:"rows" binding:"required"`
}

// handleResize handles terminal resize requests (H1: userId ownership check)
func (s *Server) handleResize(c *gin.Context) {
	sessionID := c.Param("sessionId")
	userID := c.GetHeader("X-User-ID")

	var req ResizeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	sess, err := s.getSessionWithAuth(sessionID, userID)
	if err != nil {
		if userID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "X-User-ID header is required"})
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	if err := sess.Resize(req.Cols, req.Rows); err != nil {
		log.WithError(err).Error("Failed to resize terminal")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
	})
}

// handleTerminateSession handles session termination requests (H1: userId ownership check)
func (s *Server) handleTerminateSession(c *gin.Context) {
	sessionID := c.Param("sessionId")
	userID := c.GetHeader("X-User-ID")

	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "X-User-ID header is required"})
		return
	}

	err := s.sessionManager.TerminateSessionForUser(sessionID, userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "session terminated successfully",
	})
}

// H10: handleListSessions returns sessions for the authenticated user.
func (s *Server) handleListSessions(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "X-User-ID header is required"})
		return
	}

	sessions := s.sessionManager.ListSessionsForUser(userID)
	c.JSON(http.StatusOK, gin.H{
		"sessions": sessions,
	})
}

// getSessionWithAuth returns a session, always enforcing ownership via X-User-ID.
func (s *Server) getSessionWithAuth(sessionID, userID string) (*session.Session, error) {
	if userID == "" {
		return nil, fmt.Errorf("X-User-ID header is required")
	}
	return s.sessionManager.GetSessionForUser(sessionID, userID)
}
