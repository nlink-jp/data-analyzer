// Package llm provides an OpenAI-compatible LLM client.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/nlink-jp/nlk/backoff"
	"github.com/nlink-jp/nlk/strip"
)

// Client is the interface for LLM interaction, enabling testability.
type Client interface {
	Chat(ctx context.Context, system, user string) (string, error)
	WaitForModel(ctx context.Context) error
}

// ClientConfig holds retry and health-check settings.
type ClientConfig struct {
	MaxRetries             int           // max retry attempts (default: 10)
	MaxBackoff             time.Duration // max backoff duration (default: 120s)
	HealthCheckInterval    time.Duration // polling interval for model readiness (default: 10s)
	HealthCheckTimeout     time.Duration // max wait for model readiness (default: 300s)
}

// DefaultClientConfig returns sensible defaults for local LLM backends.
func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		MaxRetries:          10,
		MaxBackoff:          120 * time.Second,
		HealthCheckInterval: 10 * time.Second,
		HealthCheckTimeout:  300 * time.Second,
	}
}

// HTTPClient implements Client using the OpenAI-compatible chat completions API.
type HTTPClient struct {
	endpoint string
	model    string
	apiKey   string
	http     *http.Client
	cfg      ClientConfig
}

// NewHTTPClient creates a new OpenAI-compatible API client.
func NewHTTPClient(endpoint, model, apiKey string, cfg ClientConfig) *HTTPClient {
	return &HTTPClient{
		endpoint: endpoint,
		model:    model,
		apiKey:   apiKey,
		http: &http.Client{
			Timeout: 5 * time.Minute, // local LLM inference can be slow
		},
		cfg: cfg,
	}
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// modelsResponse represents the /v1/models API response.
type modelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// WaitForModel polls the /v1/models endpoint until the configured model appears
// or the health-check timeout is reached. This detects model unloading/crashing
// and waits for the backend to reload.
func (c *HTTPClient) WaitForModel(ctx context.Context) error {
	if c.cfg.HealthCheckTimeout <= 0 {
		return nil
	}

	deadline := time.Now().Add(c.cfg.HealthCheckTimeout)
	url := strings.TrimRight(c.endpoint, "/") + "/models"
	interval := c.cfg.HealthCheckInterval
	if interval <= 0 {
		interval = 10 * time.Second
	}

	// Use a short-timeout HTTP client for health checks
	hc := &http.Client{Timeout: 10 * time.Second}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("creating health-check request: %w", err)
		}
		if c.apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+c.apiKey)
		}

		resp, err := hc.Do(req)
		if err == nil {
			body, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if readErr == nil && resp.StatusCode == http.StatusOK {
				var models modelsResponse
				if json.Unmarshal(body, &models) == nil {
					for _, m := range models.Data {
						if m.ID == c.model {
							return nil // model is ready
						}
					}
				}
			}
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("health-check timeout: model %q not available after %v", c.model, c.cfg.HealthCheckTimeout)
		}

		log.Printf("Waiting for model %q to become ready (next check in %v)...", c.model, interval.Round(time.Second))

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}

// Chat sends a system+user message pair and returns the LLM response text.
// Retries on transient errors with exponential backoff.
// On model crash/unload errors, waits for model readiness before retrying.
// Strips thinking/reasoning tags from the response.
func (c *HTTPClient) Chat(ctx context.Context, system, user string) (string, error) {
	reqBody := chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
	}

	bo := backoff.New(
		backoff.WithBase(2*time.Second),
		backoff.WithMax(c.cfg.MaxBackoff),
	)

	maxRetries := c.cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 10
	}

	var lastErr error
	for attempt := range maxRetries + 1 {
		text, err := c.callAPI(ctx, reqBody)
		if err == nil {
			return strip.ThinkTags(text), nil
		}

		lastErr = err
		errStr := strings.ToLower(err.Error())

		retryable := isRetryableError(errStr)
		if !retryable || attempt == maxRetries {
			return "", fmt.Errorf("LLM call failed: %w", err)
		}

		// If it looks like a model crash/unload, wait for model readiness
		if isModelCrashError(errStr) {
			log.Printf("Model crash detected (attempt %d/%d): %v", attempt+1, maxRetries+1, err)
			log.Printf("Waiting for model to reload...")
			if waitErr := c.WaitForModel(ctx); waitErr != nil {
				return "", fmt.Errorf("model did not recover: %w (original: %w)", waitErr, err)
			}
			log.Printf("Model is ready, retrying...")
			continue
		}

		wait := bo.Duration(attempt)
		log.Printf("LLM call failed (attempt %d/%d), retrying in %v: %v",
			attempt+1, maxRetries+1, wait.Round(time.Second), err)

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(wait):
		}
	}

	return "", fmt.Errorf("LLM call failed after %d retries: %w", maxRetries, lastErr)
}

// isRetryableError returns true if the error string indicates a transient failure.
func isRetryableError(errStr string) bool {
	retryablePatterns := []string{
		"429", "503", "500",
		"timeout", "connection refused", "eof",
		"crashed", "model not found",
	}
	for _, k := range retryablePatterns {
		if strings.Contains(errStr, k) {
			return true
		}
	}
	return false
}

// isModelCrashError returns true if the error indicates the model crashed or was unloaded.
func isModelCrashError(errStr string) bool {
	crashPatterns := []string{"crashed", "model not found", "unloaded"}
	for _, k := range crashPatterns {
		if strings.Contains(errStr, k) {
			return true
		}
	}
	return false
}

func (c *HTTPClient) callAPI(ctx context.Context, reqBody chatRequest) (string, error) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	url := strings.TrimRight(c.endpoint, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("parsing API response: %w", err)
	}

	if chatResp.Error != nil {
		return "", fmt.Errorf("API error: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("empty response from LLM")
	}

	return chatResp.Choices[0].Message.Content, nil
}
