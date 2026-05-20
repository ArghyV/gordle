// Package gordle provides core type definitions and interfaces for the gordle system.
// It does not implement any concrete logic; implementations reside in sub‑packages.
package gordle

import (
	"context"
)

// Options configures the execution of the gordle CLI.
// Fields:
//   - YoloMode: if true, task confirmations are skipped.
//   - MaxRetries: maximum number of retry attempts for each task.
//   - PlanFile: path to the plan file (required by the CLI).
type Options struct {
	YoloMode   bool
	MaxRetries int
	PlanFile   string
}

// ArtifactReference describes an artifact by its reference name and optional type.
// The Type field is used for display purposes; it may be empty.
type ArtifactReference struct {
	Ref  string
	Type string
}

// TaskSpec defines a single unit of work for the gordle system.
// Fields:
//   - ID: unique identifier of the task.
//   - Package: name of the target package.
//   - ArtifactIn: list of input artifacts required by the task.
//   - ArtifactOut: list of output artifacts produced by the task.
//   - DependsOn: identifiers of tasks that must complete before this task.
type TaskSpec struct {
	ID          string
	Package     string
	ArtifactIn  []ArtifactReference
	ArtifactOut []ArtifactReference
	DependsOn   []string
}

// LLMCaller abstracts a language‑model service that can generate a response for a prompt.
// The Call method sends the prompt and returns the generated text or an error.
type LLMCaller interface {
	// Call sends the prompt to the language model and returns the generated response.
	// ctx may be used to control cancellation; prompt is the text to be processed.
	Call(ctx context.Context, prompt string) (string, error)
}

// ArtifactStore abstracts a versioned artifact repository.
// Implementations store artifacts under a reference name and integer version.
type ArtifactStore interface {
	// Promote stores the given content under the specified reference and version.
	// It must return an error if the artifact already exists or on I/O failure.
	Promote(ref string, version int, content []byte) error
	// Get retrieves the content for the specified reference and version.
	// It returns an error if the artifact does not exist or on I/O failure.
	Get(ref string, version int) ([]byte, error)
}

// Validator abstracts a command‑execution environment that can validate generated output.
// The Validate method runs the provided command and returns whether it succeeded,
// the combined stdout/stderr output, and any error encountered while setting up or running.
type Validator interface {
	// Validate runs the given command in the specified working directory.
	// It returns true if the command exits with status 0, false otherwise,
	// along with the command's output and any error that occurred.
	Validate(cmd string, workdir string) (passed bool, output string, err error)
}
