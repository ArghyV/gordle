// Package llm provides utilities for interacting with language model APIs and managing prompt caching.
// It does not implement any higher‑level application logic.
package llm

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// workerSystemPromptPath is the well-known location of the worker system prompt.
// The file must exist at this path relative to the working directory.
const workerSystemPromptPath = "worker-system-prompt.md"

// Cerebras implements the LLMCaller interface for the Cerebras chat completions API.
type Cerebras struct {
	apiKey string
	model  string
	cache  *Cache
	client *http.Client
}

// NewCerebras creates a new Cerebras caller with the given API key, model, and cache.
// The returned value implements the LLMCaller interface.
func NewCerebras(apiKey string, model string, cache *Cache) *Cerebras {
	return &Cerebras{
		apiKey: apiKey,
		model:  model,
		cache:  cache,
		client: http.DefaultClient,
	}
}

// cerebrasRequest is the request body for the Cerebras chat completions API.
type cerebrasRequest struct {
	Model       string          `json:"model"`
    Temperature string          `json:"temperature"`
    Reasoning   string          `json:"reasoning_effort"`
	Messages []cerebrasMessage  `json:"messages"`
}

// cerebrasMessage is a single message in the Cerebras chat completions request.
type cerebrasMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// cerebrasResponse is the response body from the Cerebras chat completions API.
type cerebrasResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// Call sends the prompt to the Cerebras chat completions API and returns the response content.
// It first checks the cache; on a cache miss it performs the HTTP request and stores the result.
// Returns an error if any step fails.
func (c *Cerebras) Call(ctx context.Context, prompt string) (string, error) {
	// Compute cache key.
	hash := HashPrompt(prompt)

	// Attempt to retrieve from cache.
	cached, err := c.cache.Get(hash)
	if err == nil {
		return cached, nil
	}
	if !os.IsNotExist(err) {
		return "", fmt.Errorf("call: cache get error %w", err)
	}

	// Prepare request payload.
    workerSystemPrompt, err := os.ReadFile(workerSystemPromptPath)
	if err != nil {
		return "", fmt.Errorf("call: read worker system prompt: %w", err)
	}

	reqBody := cerebrasRequest{
		Model: c.model,
        Temperature: "0",
        Reasoning: "high",
		Messages: []cerebrasMessage{
            {Role: "system", Content: string(workerSystemPrompt)},
			{Role: "user", Content: prompt},
		},
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("call: marshal request error %w", err)
	}

	// Build HTTP request.
	const endpoint = "https://api.cerebras.ai/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return "", fmt.Errorf("call: create request error %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	// Execute request.
	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("call: request error %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("call: unexpected status %d", resp.StatusCode)
	}

	// Decode response.
	var respBody cerebrasResponse
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return "", fmt.Errorf("call: decode response error %w", err)
	}
	if len(respBody.Choices) == 0 {
		return "", fmt.Errorf("call: empty choices in response")
	}
	result := respBody.Choices[0].Message.Content

	// Store result in cache.
	if setErr := c.cache.Set(hash, result); setErr != nil {
		// Cache write failure does not block returning the result.
		return result, fmt.Errorf("call: cache set error %w", setErr)
	}
	return result, nil
}

// LoadAPIKey reads the .env file at envPath and extracts the CEREBRAS_API_KEY value.
// Returns an error if the file cannot be read or the key is missing.
func LoadAPIKey(envPath string) (string, error) {
	file, err := os.Open(envPath)
	if err != nil {
		return "", fmt.Errorf("loadapikey: open env file error %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip empty lines and comments.
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "CEREBRAS_API_KEY" {
			return value, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("loadapikey: scan env file error %w", err)
	}
	return "", fmt.Errorf("loadapikey: api key not found")
}

// EnsureGitignore makes sure that the .gitignore file in workdir contains an entry for .env.
// If the entry is missing, it appends it. The function is safe to call multiple times.
func EnsureGitignore(workdir string) error {
	gitignorePath := filepath.Join(workdir, ".gitignore")
	const envEntry = ".env"

	// Read existing .gitignore content if the file exists.
	var lines []string
	if data, err := os.ReadFile(gitignorePath); err == nil {
		lines = strings.Split(string(data), "\n")
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("ensuregitignore: read file error %w", err)
	}

	// Check if .env entry already exists.
	for _, line := range lines {
		if strings.TrimSpace(line) == envEntry {
			return nil
		}
	}

	// Append .env entry.
	f, err := os.OpenFile(gitignorePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("ensuregitignore: open file error %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(envEntry + "\n"); err != nil {
		return fmt.Errorf("ensuregitignore: write entry error %w", err)
	}
	return nil
}
