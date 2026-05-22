// Package types provides shared type definitions and interface contracts for the gordle system. It does not implement any functionality.
package types

import (
	"context"
)

// LLMCaller defines an interface for calling a language model with a prompt.
type LLMCaller interface {
	// Call sends the prompt to the language model and returns the generated response.
	// Returns an error if the call fails.
	Call(ctx context.Context, prompt string) (string, error)
}

// ArtifactStore defines an interface for storing and retrieving versioned artifacts.
type ArtifactStore interface {
	// Promote stores the given content under the specified reference and version.
	// Returns an error if the promotion fails.
	Promote(ref string, version int, content []byte) error
	// Get retrieves the content for the specified reference and version.
	// Returns an error if the retrieval fails.
	Get(ref string, version int) ([]byte, error)
}

// Validator defines an interface for validating commands within a working directory.
type Validator interface {
	// Validate runs the command in the given workdir and returns whether it passed,
	// the command's output, and any error encountered.
	Validate(cmd string, workdir string) (passed bool, output string, err error)
}

// TaskSpec describes a task's metadata, dependencies, and required signatures.
type TaskSpec struct {
	// ID is the unique identifier of the task.
	ID string `yaml:"id"`
	// Title is a short human‑readable title for the task.
	Title string `yaml:"title"`
	// Package is the Go package name where the task's code will reside.
	Package string `yaml:"package"`
	// ArtifactIn lists input artifacts required by the task.
	ArtifactIn []ArtifactRef `yaml:"artifact_in"`
	// ArtifactOut lists output artifacts produced by the task.
	ArtifactOut []ArtifactRef `yaml:"artifact_out"`
	// DependsOn lists IDs of tasks that must be completed before this task.
	DependsOn []string `yaml:"depends_on"`
	// SymbolsRequired lists symbols that must be available for the task.
	SymbolsRequired []SymbolRef `yaml:"symbols_required"`
	// FunctionSignatures lists the function signatures that the task must implement.
	FunctionSignatures []string `yaml:"function_signatures"`
	// InterfaceImpl specifies the name of an interface that the task implements, if any.
	InterfaceImpl string `yaml:"interface_impl"`
	// ErrorConvention describes the error handling policy for the task.
	ErrorConvention string `yaml:"error_convention"`
	// ValidationCmd is the command used to validate the task's generated code.
	ValidationCmd string `yaml:"validation_cmd"`
	// Notes contains any additional information about the task.
	Notes string `yaml:"notes"`
}

// ArtifactRef describes a reference to an artifact, including its type and identifier.
type ArtifactRef struct {
	// Type indicates the kind of artifact (e.g., "code", "data").
	Type string `yaml:"type"`
	// Ref is the unique reference string for the artifact.
	Ref string `yaml:"ref"`
}

// SymbolRef describes a required symbol, including its name and location.
type SymbolRef struct {
	// Symbol is the name of the required symbol.
	Symbol string `yaml:"symbol"`
	// Location indicates where the symbol is defined (e.g., package path).
	Location string `yaml:"location"`
}

// Options configures execution parameters for task processing.
type Options struct {
	// YoloMode enables aggressive behavior that may skip safety checks.
	YoloMode bool
	// MaxRetries specifies the maximum number of retry attempts for transient failures.
	MaxRetries int
	// PlanFile is the path to the plan file describing the task workflow.
	PlanFile string
}
