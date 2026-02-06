package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	log "github.com/sirupsen/logrus"

	"github.com/servicenow/claude-terminal-mid-service/internal/config"
	"github.com/servicenow/claude-terminal-mid-service/internal/logging"
	"github.com/servicenow/claude-terminal-mid-service/internal/servicenow"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Warn("No .env file found, using environment variables")
	}

	// Initialize configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// H7: Deduplicated shared logging setup (C5: file perms 0640 handled inside)
	logging.Setup(cfg)

	log.Info("Starting ECC Queue Poller")
	log.Infof("ServiceNow Instance: %s", cfg.ServiceNow.Instance)
	log.Infof("Node Service: http://%s:%d", cfg.Server.Host, cfg.Server.Port)

	// Initialize ServiceNow client
	snClient := servicenow.NewClient(cfg)

	// Initialize Node service client
	nodeClient := servicenow.NewNodeServiceClient(cfg)

	// Create poller
	poller := NewECCPoller(cfg, snClient, nodeClient)

	// Start polling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go poller.Start(ctx)

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down ECC Queue Poller...")
	cancel()

	// Give some time for graceful shutdown
	time.Sleep(2 * time.Second)

	log.Info("ECC Queue Poller stopped")
}

// ECCPoller polls the ECC Queue for commands
type ECCPoller struct {
	config     *config.Config
	snClient   *servicenow.Client
	nodeClient *servicenow.NodeServiceClient
	interval   time.Duration
}

// NewECCPoller creates a new ECC Queue poller
func NewECCPoller(cfg *config.Config, snClient *servicenow.Client, nodeClient *servicenow.NodeServiceClient) *ECCPoller {
	return &ECCPoller{
		config:     cfg,
		snClient:   snClient,
		nodeClient: nodeClient,
		interval:   5 * time.Second, // Poll every 5 seconds
	}
}

// Start starts the polling loop
func (p *ECCPoller) Start(ctx context.Context) {
	log.Info("ECC Queue Poller started")

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("ECC Queue Poller stopping...")
			return
		case <-ticker.C:
			if err := p.poll(ctx); err != nil {
				log.WithError(err).Error("Polling error")
			}
		}
	}
}

// poll performs a single poll cycle
func (p *ECCPoller) poll(ctx context.Context) error {
	// Get pending ECC Queue items
	items, err := p.snClient.GetECCQueueItems(ctx)
	if err != nil {
		return fmt.Errorf("failed to get ECC queue items: %w", err)
	}

	if len(items) == 0 {
		return nil
	}

	log.Infof("Processing %d ECC Queue items", len(items))

	// H4: Process items concurrently with a worker pool (max 5 workers)
	const maxWorkers = 5
	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup

	for _, item := range items {
		wg.Add(1)
		sem <- struct{}{} // Acquire worker slot
		go func(it servicenow.ECCQueueItem) {
			defer wg.Done()
			defer func() { <-sem }() // Release worker slot

			// H4: Per-item context timeout
			itemCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			if err := p.processItem(itemCtx, it); err != nil {
				log.WithError(err).WithField("sys_id", it.SysID).Error("Failed to process item")
			}
		}(item)
	}

	wg.Wait()
	return nil
}

// processItem processes a single ECC Queue item
func (p *ECCPoller) processItem(ctx context.Context, item servicenow.ECCQueueItem) error {
	log.WithFields(log.Fields{
		"sys_id": item.SysID,
		"name":   item.Name,
	}).Info("Processing ECC Queue item")

	// Update to processing state
	if err := p.snClient.UpdateECCQueueItem(ctx, item.SysID, "processing", ""); err != nil {
		return fmt.Errorf("failed to update to processing: %w", err)
	}

	// Parse payload
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(item.Payload), &payload); err != nil {
		updateErr := p.snClient.UpdateECCQueueItem(ctx, item.SysID, "error", fmt.Sprintf("Invalid payload: %v", err))
		if updateErr != nil {
			log.WithError(updateErr).WithField("sys_id", item.SysID).Error("Failed to update item to error state")
		}
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	// H3: Handle type assertion failure for action
	action, ok := payload["action"].(string)
	if !ok {
		errMsg := "missing or invalid 'action' field in payload"
		updateErr := p.snClient.UpdateECCQueueItem(ctx, item.SysID, "error", errMsg)
		if updateErr != nil {
			log.WithError(updateErr).WithField("sys_id", item.SysID).Error("Failed to update item to error state")
		}
		return fmt.Errorf("%s", errMsg)
	}

	var result interface{}
	var processErr error

	switch action {
	case "create_session":
		result, processErr = p.handleCreateSession(ctx, payload)
	case "send_command":
		result, processErr = p.handleSendCommand(ctx, payload)
	case "get_output":
		result, processErr = p.handleGetOutput(ctx, payload)
	case "get_status":
		result, processErr = p.handleGetStatus(ctx, payload)
	case "terminate_session":
		result, processErr = p.handleTerminateSession(ctx, payload)
	case "resize_terminal":
		result, processErr = p.handleResizeTerminal(ctx, payload)
	default:
		processErr = fmt.Errorf("unknown action: %s", action)
	}

	// Update ECC Queue item based on result
	if processErr != nil {
		updateErr := p.snClient.UpdateECCQueueItem(ctx, item.SysID, "error", processErr.Error())
		if updateErr != nil {
			log.WithError(updateErr).WithField("sys_id", item.SysID).Error("Failed to update item to error state")
		}
		return processErr
	}

	// H3: Handle json.Marshal error
	resultJSON, err := json.Marshal(result)
	if err != nil {
		log.WithError(err).WithField("sys_id", item.SysID).Error("Failed to marshal result")
		updateErr := p.snClient.UpdateECCQueueItem(ctx, item.SysID, "error", fmt.Sprintf("failed to marshal result: %v", err))
		if updateErr != nil {
			log.WithError(updateErr).WithField("sys_id", item.SysID).Error("Failed to update item to error state")
		}
		return fmt.Errorf("failed to marshal result: %w", err)
	}

	if err := p.snClient.UpdateECCQueueItem(ctx, item.SysID, "processed", string(resultJSON)); err != nil {
		log.WithError(err).WithField("sys_id", item.SysID).Error("Failed to update item to processed state")
	}

	// Create response in output queue
	if err := p.snClient.CreateECCQueueResponse(ctx, item, result, nil); err != nil {
		log.WithError(err).WithField("sys_id", item.SysID).Error("Failed to create ECC queue response")
	}

	log.WithField("sys_id", item.SysID).Info("Successfully processed ECC Queue item")
	return nil
}

func (p *ECCPoller) handleCreateSession(ctx context.Context, payload map[string]interface{}) (interface{}, error) {
	// H3: Validate type assertions
	userID, ok := payload["userId"].(string)
	if !ok || userID == "" {
		return nil, fmt.Errorf("missing or invalid 'userId' in payload")
	}
	workspaceType, _ := payload["workspaceType"].(string)

	credMap, ok := payload["credentials"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'credentials' in payload")
	}
	apiKey, ok := credMap["anthropicApiKey"].(string)
	if !ok || apiKey == "" {
		return nil, fmt.Errorf("missing or invalid 'anthropicApiKey' in credentials")
	}
	githubToken, _ := credMap["githubToken"].(string)

	return p.nodeClient.CreateSession(ctx, userID, apiKey, githubToken, workspaceType)
}

func (p *ECCPoller) handleSendCommand(ctx context.Context, payload map[string]interface{}) (interface{}, error) {
	sessionID, ok := payload["sessionId"].(string)
	if !ok || sessionID == "" {
		return nil, fmt.Errorf("missing or invalid 'sessionId' in payload")
	}
	command, ok := payload["command"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'command' in payload")
	}

	return p.nodeClient.SendCommand(ctx, sessionID, command)
}

func (p *ECCPoller) handleGetOutput(ctx context.Context, payload map[string]interface{}) (interface{}, error) {
	sessionID, ok := payload["sessionId"].(string)
	if !ok || sessionID == "" {
		return nil, fmt.Errorf("missing or invalid 'sessionId' in payload")
	}
	clear, _ := payload["clear"].(bool)

	return p.nodeClient.GetOutput(ctx, sessionID, clear)
}

func (p *ECCPoller) handleGetStatus(ctx context.Context, payload map[string]interface{}) (interface{}, error) {
	sessionID, ok := payload["sessionId"].(string)
	if !ok || sessionID == "" {
		return nil, fmt.Errorf("missing or invalid 'sessionId' in payload")
	}

	return p.nodeClient.GetStatus(ctx, sessionID)
}

func (p *ECCPoller) handleTerminateSession(ctx context.Context, payload map[string]interface{}) (interface{}, error) {
	sessionID, ok := payload["sessionId"].(string)
	if !ok || sessionID == "" {
		return nil, fmt.Errorf("missing or invalid 'sessionId' in payload")
	}

	return p.nodeClient.TerminateSession(ctx, sessionID)
}

func (p *ECCPoller) handleResizeTerminal(ctx context.Context, payload map[string]interface{}) (interface{}, error) {
	sessionID, ok := payload["sessionId"].(string)
	if !ok || sessionID == "" {
		return nil, fmt.Errorf("missing or invalid 'sessionId' in payload")
	}
	cols, ok := payload["cols"].(float64)
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'cols' in payload")
	}
	rows, ok := payload["rows"].(float64)
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'rows' in payload")
	}

	return p.nodeClient.ResizeTerminal(ctx, sessionID, int(cols), int(rows))
}
