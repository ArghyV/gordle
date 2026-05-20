// Package llm provides a simple SHA‑256 keyed JSON cache for language model prompts.
// It does not implement any LLM calling logic.
package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
)

// Cache stores responses keyed by the SHA‑256 hash of the prompt.
type Cache struct {
	dir string
}

// NewCache creates a new Cache that stores files under the given cacheDir.
// The cache directory is not created until needed.
func NewCache(cacheDir string) *Cache {
	return &Cache{dir: cacheDir}
}

// Get retrieves the cached response for the given promptHash.
// It returns the response string and nil on success.
// If the cache entry does not exist, it returns an empty string and os.ErrNotExist.
func (c *Cache) Get(promptHash string) (string, error) {
	var response string
	var err error

	// Build the full path to the JSON file.
	var cachePath string = filepath.Join(c.dir, ".harness", "cache", promptHash+".json")

	// Open the file for reading.
	var file *os.File
	file, err = os.Open(cachePath)
	if err != nil {
		// Propagate the error (including os.ErrNotExist) to the caller.
		return "", err
	}
	defer file.Close()

	// Decode the JSON content.
	var entry struct {
		Response string `json:"response"`
	}
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&entry)
	if err != nil {
		return "", err
	}
	response = entry.Response
	return response, nil
}

// Set stores the response string in the cache under the given promptHash.
// The response is written as JSON. Any error encountered while writing is returned.
func (c *Cache) Set(promptHash string, response string) error {
	// Ensure the target directory exists.
	var cacheDir string = filepath.Join(c.dir, ".harness", "cache")
	err := os.MkdirAll(cacheDir, 0o755)
	if err != nil {
		return err
	}

	// Build the full path to the JSON file.
	var cachePath string = filepath.Join(cacheDir, promptHash+".json")

	// Prepare the JSON payload.
	var entry struct {
		Response string `json:"response"`
	}
	entry.Response = response

	// Marshal the JSON.
	var data []byte
	data, err = json.Marshal(entry)
	if err != nil {
		return err
	}

	// Write the file atomically.
	var tmpPath string = cachePath + ".tmp"
	var tmpFile *os.File
	tmpFile, err = os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, err = tmpFile.Write(data)
	if closeErr := tmpFile.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	// Rename the temporary file to the final name.
	err = os.Rename(tmpPath, cachePath)
	if err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

// HashPrompt returns the hexadecimal SHA‑256 hash of the given prompt string.
func HashPrompt(prompt string) string {
	var hasher = sha256.New()
	_, _ = hasher.Write([]byte(prompt))
	var hashBytes []byte = hasher.Sum(nil)
	return hex.EncodeToString(hashBytes)
}
