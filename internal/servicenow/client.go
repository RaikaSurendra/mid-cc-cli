package servicenow

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/servicenow/claude-terminal-mid-service/internal/config"
)

// Client is a ServiceNow API client
type Client struct {
	config     *config.Config
	httpClient *http.Client
	baseURL    string
	auth       string
}

// ECCQueueItem represents an ECC Queue item
type ECCQueueItem struct {
	SysID   string `json:"sys_id"`
	Topic   string `json:"topic"`
	Name    string `json:"name"`
	Queue   string `json:"queue"`
	State   string `json:"state"`
	Payload string `json:"payload"`
	Source  string `json:"source"`
}

// NewClient creates a new ServiceNow client
func NewClient(cfg *config.Config) *Client {
	auth := base64.StdEncoding.EncodeToString(
		[]byte(fmt.Sprintf("%s:%s", cfg.ServiceNow.Username, cfg.ServiceNow.Password)),
	)

	return &Client{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: fmt.Sprintf("https://%s", cfg.ServiceNow.Instance),
		auth:    auth,
	}
}

// GetECCQueueItems gets pending ECC Queue items
func (c *Client) GetECCQueueItems(ctx context.Context) ([]ECCQueueItem, error) {
	query := url.Values{}
	query.Set("sysparm_query", "topic=ClaudeTerminalCommand^state=ready")
	query.Set("sysparm_limit", "10")

	endpoint := fmt.Sprintf("%s/api/now/table/ecc_queue?%s", c.baseURL, query.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Basic %s", c.auth))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result struct {
		Result []ECCQueueItem `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Result, nil
}

// UpdateECCQueueItem updates an ECC Queue item
func (c *Client) UpdateECCQueueItem(ctx context.Context, sysID, state, output string) error {
	data := map[string]interface{}{
		"state":     state,
		"output":    output,
		"processed": time.Now().Format(time.RFC3339),
	}

	endpoint := fmt.Sprintf("%s/api/now/table/ecc_queue/%s", c.baseURL, sysID)

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "PATCH", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Basic %s", c.auth))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

// A5: Fixed variable shadowing - renamed inner err to marshalErr / reqErr etc.
// CreateECCQueueResponse creates a response in the ECC Queue
func (c *Client) CreateECCQueueResponse(ctx context.Context, originalItem ECCQueueItem, output interface{}, responseErr error) error {
	state := "ready"
	if responseErr != nil {
		state = "error"
	}

	outputJSON, marshalErr := json.Marshal(map[string]interface{}{
		"success":   responseErr == nil,
		"data":      output,
		"error":     fmt.Sprintf("%v", responseErr),
		"timestamp": time.Now().Format(time.RFC3339),
	})
	if marshalErr != nil {
		return fmt.Errorf("failed to marshal output JSON: %w", marshalErr)
	}

	data := map[string]interface{}{
		"topic":  "ClaudeTerminalResponse",
		"queue":  "output",
		"state":  state,
		"name":   originalItem.Name,
		"source": originalItem.Source,
		"output": string(outputJSON),
	}

	endpoint := fmt.Sprintf("%s/api/now/table/ecc_queue", c.baseURL)

	jsonData, marshalErr := json.Marshal(data)
	if marshalErr != nil {
		return fmt.Errorf("failed to marshal request data: %w", marshalErr)
	}

	req, reqErr := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(jsonData))
	if reqErr != nil {
		return reqErr
	}

	req.Header.Set("Authorization", fmt.Sprintf("Basic %s", c.auth))
	req.Header.Set("Content-Type", "application/json")

	resp, doErr := c.httpClient.Do(req)
	if doErr != nil {
		return doErr
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

// NodeServiceClient is a client for the local Node service
type NodeServiceClient struct {
	config     *config.Config
	httpClient *http.Client
	baseURL    string
}

// NewNodeServiceClient creates a new Node service client.
// H5: Uses HTTPS when TLS certs are configured.
func NewNodeServiceClient(cfg *config.Config) *NodeServiceClient {
	scheme := "http"
	if cfg.Security.TLSCertPath != "" && cfg.Security.TLSKeyPath != "" {
		scheme = "https"
	}

	return &NodeServiceClient{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: fmt.Sprintf("%s://%s:%d", scheme, cfg.Server.Host, cfg.Server.Port),
	}
}

// CreateSession creates a new terminal session
func (c *NodeServiceClient) CreateSession(ctx context.Context, userID, apiKey, githubToken, workspaceType string) (interface{}, error) {
	data := map[string]interface{}{
		"userId": userID,
		"credentials": map[string]string{
			"anthropicApiKey": apiKey,
			"githubToken":     githubToken,
		},
		"workspaceType": workspaceType,
	}

	return c.makeRequest(ctx, "POST", "/api/session/create", data)
}

// SendCommand sends a command to a session
func (c *NodeServiceClient) SendCommand(ctx context.Context, sessionID, command string) (interface{}, error) {
	data := map[string]interface{}{
		"command": command,
	}

	return c.makeRequest(ctx, "POST", fmt.Sprintf("/api/session/%s/command", sessionID), data)
}

// GetOutput gets session output
func (c *NodeServiceClient) GetOutput(ctx context.Context, sessionID string, clear bool) (interface{}, error) {
	endpoint := fmt.Sprintf("/api/session/%s/output?clear=%t", sessionID, clear)
	return c.makeRequest(ctx, "GET", endpoint, nil)
}

// GetStatus gets session status
func (c *NodeServiceClient) GetStatus(ctx context.Context, sessionID string) (interface{}, error) {
	return c.makeRequest(ctx, "GET", fmt.Sprintf("/api/session/%s/status", sessionID), nil)
}

// TerminateSession terminates a session
func (c *NodeServiceClient) TerminateSession(ctx context.Context, sessionID string) (interface{}, error) {
	return c.makeRequest(ctx, "DELETE", fmt.Sprintf("/api/session/%s", sessionID), nil)
}

// ResizeTerminal resizes a terminal
func (c *NodeServiceClient) ResizeTerminal(ctx context.Context, sessionID string, cols, rows int) (interface{}, error) {
	data := map[string]interface{}{
		"cols": cols,
		"rows": rows,
	}

	return c.makeRequest(ctx, "POST", fmt.Sprintf("/api/session/%s/resize", sessionID), data)
}

func (c *NodeServiceClient) makeRequest(ctx context.Context, method, endpoint string, data interface{}) (interface{}, error) {
	var body []byte
	var err error

	if data != nil {
		body, err = json.Marshal(data)
		if err != nil {
			return nil, err
		}
	}

	reqURL := fmt.Sprintf("%s%s", c.baseURL, endpoint)
	log.Debugf("Node service request: %s %s", method, reqURL)

	var req *http.Request
	if body != nil {
		req, err = http.NewRequestWithContext(ctx, method, reqURL, bytes.NewBuffer(body))
	} else {
		req, err = http.NewRequestWithContext(ctx, method, reqURL, nil)
	}

	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	// Forward API auth token if configured
	if c.config.Security.APIAuthToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.config.Security.APIAuthToken))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result, nil
}
