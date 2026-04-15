package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHTTPClientChat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %s, want /chat/completions", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}

		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Model != "test-model" {
			t.Errorf("model = %q, want test-model", req.Model)
		}
		if len(req.Messages) != 2 {
			t.Errorf("messages = %d, want 2", len(req.Messages))
		}

		resp := chatResponse{
			Choices: []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			}{
				{Message: struct {
					Content string `json:"content"`
				}{Content: "test response"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "test-model", "", DefaultClientConfig())
	text, err := client.Chat(context.Background(), "system", "user")
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if text != "test response" {
		t.Errorf("response = %q, want %q", text, "test response")
	}
}

func TestHTTPClientChatWithAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-key" {
			t.Errorf("Authorization = %q, want Bearer test-key", auth)
		}

		resp := chatResponse{
			Choices: []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			}{
				{Message: struct {
					Content string `json:"content"`
				}{Content: "ok"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "model", "test-key", DefaultClientConfig())
	_, err := client.Chat(context.Background(), "sys", "usr")
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
}

func TestHTTPClientChatStripsThinkTags(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := chatResponse{
			Choices: []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			}{
				{Message: struct {
					Content string `json:"content"`
				}{Content: "<think>reasoning here</think>actual response"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "model", "", DefaultClientConfig())
	text, err := client.Chat(context.Background(), "sys", "usr")
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if strings.Contains(text, "<think>") {
		t.Errorf("response still contains think tags: %q", text)
	}
	if !strings.Contains(text, "actual response") {
		t.Errorf("response missing expected content: %q", text)
	}
}

func TestHTTPClientChatAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
	}))
	defer server.Close()

	cfg := DefaultClientConfig()
	cfg.MaxRetries = 0 // no retries for this test
	client := NewHTTPClient(server.URL, "model", "", cfg)
	_, err := client.Chat(context.Background(), "sys", "usr")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error = %q, want to contain 400", err.Error())
	}
}

func TestHTTPClientChatRetriesOnModelCrash(t *testing.T) {
	var calls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)

		if r.URL.Path == "/models" {
			// Health check: model available from the start
			resp := modelsResponse{
				Data: []struct {
					ID string `json:"id"`
				}{{ID: "test-model"}},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}

		if n <= 2 {
			// First two chat calls: model crash
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"The model has crashed without additional information. (Exit code: null)"}`))
			return
		}

		// Third call: success
		resp := chatResponse{
			Choices: []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			}{
				{Message: struct {
					Content string `json:"content"`
				}{Content: "recovered"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := ClientConfig{
		MaxRetries:          5,
		MaxBackoff:          1 * time.Second,
		HealthCheckInterval: 100 * time.Millisecond,
		HealthCheckTimeout:  5 * time.Second,
	}
	client := NewHTTPClient(server.URL, "test-model", "", cfg)
	text, err := client.Chat(context.Background(), "sys", "usr")
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if text != "recovered" {
		t.Errorf("response = %q, want %q", text, "recovered")
	}
}

func TestHTTPClientChatRetriesOn500(t *testing.T) {
	var calls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("internal error"))
			return
		}
		resp := chatResponse{
			Choices: []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			}{
				{Message: struct {
					Content string `json:"content"`
				}{Content: "ok"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := ClientConfig{
		MaxRetries: 3,
		MaxBackoff: 100 * time.Millisecond,
	}
	client := NewHTTPClient(server.URL, "model", "", cfg)
	text, err := client.Chat(context.Background(), "sys", "usr")
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if text != "ok" {
		t.Errorf("response = %q, want ok", text)
	}
}

func TestWaitForModelSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := modelsResponse{
			Data: []struct {
				ID string `json:"id"`
			}{{ID: "test-model"}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := ClientConfig{
		HealthCheckInterval: 100 * time.Millisecond,
		HealthCheckTimeout:  5 * time.Second,
	}
	client := NewHTTPClient(server.URL, "test-model", "", cfg)
	err := client.WaitForModel(context.Background())
	if err != nil {
		t.Fatalf("WaitForModel: %v", err)
	}
}

func TestWaitForModelWaitsForReload(t *testing.T) {
	var calls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)

		var models []struct {
			ID string `json:"id"`
		}
		if n >= 3 {
			// Model becomes available on 3rd poll
			models = append(models, struct {
				ID string `json:"id"`
			}{ID: "test-model"})
		}

		resp := modelsResponse{Data: models}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := ClientConfig{
		HealthCheckInterval: 50 * time.Millisecond,
		HealthCheckTimeout:  5 * time.Second,
	}
	client := NewHTTPClient(server.URL, "test-model", "", cfg)
	err := client.WaitForModel(context.Background())
	if err != nil {
		t.Fatalf("WaitForModel: %v", err)
	}
	if calls.Load() < 3 {
		t.Errorf("expected at least 3 polls, got %d", calls.Load())
	}
}

func TestWaitForModelTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always return empty model list
		resp := modelsResponse{}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := ClientConfig{
		HealthCheckInterval: 50 * time.Millisecond,
		HealthCheckTimeout:  200 * time.Millisecond,
	}
	client := NewHTTPClient(server.URL, "test-model", "", cfg)
	err := client.WaitForModel(context.Background())
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("error = %q, want to contain timeout", err.Error())
	}
}

func TestWaitForModelContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := modelsResponse{}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := ClientConfig{
		HealthCheckInterval: 50 * time.Millisecond,
		HealthCheckTimeout:  10 * time.Second,
	}
	client := NewHTTPClient(server.URL, "test-model", "", cfg)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := client.WaitForModel(ctx)
	if err == nil {
		t.Fatal("expected context error, got nil")
	}
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		errStr string
		want   bool
	}{
		{"api returned 429: rate limited", true},
		{"api returned 503: service unavailable", true},
		{"api returned 500: internal error", true},
		{"timeout exceeded", true},
		{"connection refused", true},
		{"unexpected eof", true},
		{"the model has crashed", true},
		{"model not found", true},
		{"api returned 400: bad request", false},
		{"api returned 401: unauthorized", false},
		{"parsing error", false},
	}

	for _, tt := range tests {
		t.Run(tt.errStr, func(t *testing.T) {
			got := isRetryableError(tt.errStr)
			if got != tt.want {
				t.Errorf("isRetryableError(%q) = %v, want %v", tt.errStr, got, tt.want)
			}
		})
	}
}

func TestIsModelCrashError(t *testing.T) {
	tests := []struct {
		errStr string
		want   bool
	}{
		{"the model has crashed without additional information", true},
		{"model not found", true},
		{"model has been unloaded", true},
		{"api returned 500: internal error", false},
		{"timeout exceeded", false},
	}

	for _, tt := range tests {
		t.Run(tt.errStr, func(t *testing.T) {
			got := isModelCrashError(tt.errStr)
			if got != tt.want {
				t.Errorf("isModelCrashError(%q) = %v, want %v", tt.errStr, got, tt.want)
			}
		})
	}
}
