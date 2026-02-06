package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all configuration for the service
type Config struct {
	ServiceNow ServiceNowConfig
	Server     ServerConfig
	Session    SessionConfig
	Workspace  WorkspaceConfig
	Logging    LoggingConfig
	Security   SecurityConfig
	Database   DatabaseConfig
}

// DatabaseConfig holds PostgreSQL connection configuration.
type DatabaseConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// ServiceNowConfig holds ServiceNow instance configuration
type ServiceNowConfig struct {
	Instance string
	Username string
	Password string
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Host string
	Port int
	Mode string // "debug" or "release"
}

// SessionConfig holds session management configuration
type SessionConfig struct {
	TimeoutMinutes   int
	MaxPerUser       int
	OutputBufferSize int
}

// WorkspaceConfig holds workspace configuration
type WorkspaceConfig struct {
	BasePath string
	Type     string // "isolated" or "persistent"
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level string
	File  string
}

// SecurityConfig holds security configuration
type SecurityConfig struct {
	EncryptionKey      string
	APIAuthToken       string
	CORSAllowedOrigins []string
	TLSCertPath        string
	TLSKeyPath         string
}

// Enabled returns true when a DB_HOST has been explicitly set, indicating
// the operator wants PostgreSQL-backed session persistence.
func (d DatabaseConfig) Enabled() bool {
	return os.Getenv("DB_HOST") != ""
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{
		ServiceNow: ServiceNowConfig{
			Instance: getEnv("SERVICENOW_INSTANCE", ""),
			Username: getEnv("SERVICENOW_API_USER", ""),
			Password: getEnv("SERVICENOW_API_PASSWORD", ""),
		},
		Server: ServerConfig{
			Host: getEnv("NODE_SERVICE_HOST", "localhost"),
			Port: getEnvInt("NODE_SERVICE_PORT", 3000),
			Mode: getEnv("GIN_MODE", "debug"),
		},
		Session: SessionConfig{
			TimeoutMinutes:   getEnvInt("SESSION_TIMEOUT_MINUTES", 30),
			MaxPerUser:       getEnvInt("MAX_SESSIONS_PER_USER", 3),
			OutputBufferSize: getEnvInt("OUTPUT_BUFFER_SIZE", 100),
		},
		Workspace: WorkspaceConfig{
			BasePath: getEnv("WORKSPACE_BASE_PATH", "/tmp/claude-sessions"),
			Type:     getEnv("WORKSPACE_TYPE", "isolated"),
		},
		Logging: LoggingConfig{
			Level: getEnv("LOG_LEVEL", "info"),
			File:  getEnv("LOG_FILE", ""),
		},
		Security: SecurityConfig{
			EncryptionKey:      getEnv("ENCRYPTION_KEY", ""),
			APIAuthToken:       getEnv("API_AUTH_TOKEN", ""),
			CORSAllowedOrigins: parseCORSOrigins(getEnv("CORS_ALLOWED_ORIGINS", "http://localhost")),
			TLSCertPath:        getEnv("TLS_CERT_PATH", ""),
			TLSKeyPath:         getEnv("TLS_KEY_PATH", ""),
		},
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnvInt("DB_PORT", 5432),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", ""),
			DBName:   getEnv("DB_NAME", "claude_terminal"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
	}

	// Validate required fields
	if cfg.ServiceNow.Instance == "" {
		return nil, fmt.Errorf("SERVICENOW_INSTANCE is required")
	}
	if cfg.ServiceNow.Username == "" {
		return nil, fmt.Errorf("SERVICENOW_API_USER is required")
	}
	if cfg.ServiceNow.Password == "" {
		return nil, fmt.Errorf("SERVICENOW_API_PASSWORD is required")
	}

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func parseCORSOrigins(raw string) []string {
	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			origins = append(origins, p)
		}
	}
	return origins
}
