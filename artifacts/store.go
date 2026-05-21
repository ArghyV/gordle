// Package artifacts provides a simple file‑system based versioned artifact store.
// It does not implement any higher‑level orchestration or validation logic.
package artifacts

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ArghyV/gordle/types"

)

// Store implements the types.ArtifactStore interface using the local file system.
// It stores artifacts under a root directory, creating sub‑directories as needed.
type Store struct {
	// rootDir is the base directory where artifacts are stored.
	rootDir string
}

// NewStore creates a new Store that uses the given rootDir as its base path.
// The returned Store does not perform any I/O until its methods are called.
func NewStore(rootDir string) *Store {
	return &Store{
		rootDir: rootDir,
	}
}

// Promote stores the given content under the specified reference and version.
// The artifact is written to a file named "<ref>_v<version>" inside the store's root directory.
// Returns an error if the file already exists or if writing fails.
func (s *Store) Promote(ref string, version int, content []byte) error {
	if ref == "" {
		return fmt.Errorf("promote: empty reference")
	}
	if version < 0 {
		return fmt.Errorf("promote: negative version")
	}
	// Ensure the base directory exists.
	if err := os.MkdirAll(s.rootDir, 0o755); err != nil {
		return fmt.Errorf("promote: cannot create root directory: %w", err)
	}
	artifactPath := filepath.Join(s.rootDir, fmt.Sprintf("%s_v%d", ref, version))
	// Do not overwrite existing artifacts.
	if _, err := os.Stat(artifactPath); err == nil {
		return fmt.Errorf("promote: version already exists")
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("promote: cannot stat artifact: %w", err)
	}
	if err := os.WriteFile(artifactPath, content, 0o644); err != nil {
		return fmt.Errorf("promote: cannot write artifact: %w", err)
	}
	return nil
}

// Get retrieves the content for the specified reference and version.
// It reads the file named "<ref>_v<version>" from the store's root directory.
// Returns os.ErrNotExist if the artifact does not exist.
func (s *Store) Get(ref string, version int) ([]byte, error) {
	if ref == "" {
		return nil, fmt.Errorf("get: empty reference")
	}
	if version < 0 {
		return nil, fmt.Errorf("get: negative version")
	}
	artifactPath := filepath.Join(s.rootDir, fmt.Sprintf("%s_v%d", ref, version))
	data, err := os.ReadFile(artifactPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, os.ErrNotExist
		}
		return nil, fmt.Errorf("get: cannot read artifact: %w", err)
	}
	return data, nil
}

// Ensure Store satisfies the types.ArtifactStore interface.
var _ types.ArtifactStore = (*Store)(nil)
