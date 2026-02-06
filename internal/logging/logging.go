package logging

import (
	"os"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/servicenow/claude-terminal-mid-service/internal/config"
)

// Setup configures the global logrus logger from config.
func Setup(cfg *config.Config) {
	level, err := log.ParseLevel(cfg.Logging.Level)
	if err != nil {
		level = log.InfoLevel
	}
	log.SetLevel(level)

	log.SetFormatter(&log.JSONFormatter{
		TimestampFormat: time.RFC3339,
	})

	if cfg.Logging.File != "" {
		file, err := os.OpenFile(cfg.Logging.File, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0640)
		if err != nil {
			log.Warnf("Failed to open log file %s: %v", cfg.Logging.File, err)
		} else {
			log.SetOutput(file)
		}
	}
}
