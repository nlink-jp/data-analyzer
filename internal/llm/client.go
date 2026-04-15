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

const maxRetries = 5

// Client is the interface for LLM interaction, enabling testability.
type Client interface {
	Chat(ctx context.Context, system, user string) (string, error)
}

// HTTPClient implements Client using the OpenAI-compatible chat completions API.
type HTTPClient struct {
	endpoint string
	model    string
	apiKey   string
	http     *http.Client
}

// NewHTTPClient creates a new OpenAI-compatible API client.
func NewHTTPClient(endpoint, model, apiKey string) *HTTPClient {
	return &HTTPClient{
		endpoint: endpoint,
		model:    model,
		apiKey:   apiKey,
		http: &http.Client{
			Timeout: 5 * time.Minute, // local LLM inference can be slow
		},
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

// Chat sends a system+user message pair and returns the LLM response text.
// Retries on transient errors with exponential backoff.
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
		backoff.WithMax(30*time.Second),
	)

	var lastErr error
	for attempt := range maxRetries + 1 {
		text, err := c.callAPI(ctx, reqBody)
		if err == nil {
			return strip.ThinkTags(text), nil
		}

		lastErr = err
		errStr := strings.ToLower(err.Error())
		retryable := false
		for _, k := range []string{"429", "503", "500", "timeout", "connection refused", "eof"} {
			if strings.Contains(errStr, k) {
				retryable = true
				break
			}
		}

		if !retryable || attempt == maxRetries {
			return "", fmt.Errorf("LLM call failed: %w", err)
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
