// Package queue provides functions for decomposing a plan into tasks, resolving task execution order,
// and dispatching tasks with retry, validation, and artifact promotion.
//
// It does not implement any persistence, concurrency, or external orchestration beyond the provided interfaces.
package queue

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ArghyV/gordle/types"
	"github.com/ArghyV/gordle/injector"
	"gopkg.in/yaml.v3"
)

// decompositionPromptPath is the well-known location of the decomposition instruction set.
// The file must exist at this path relative to the working directory.
const decompositionPromptPath = "decomposition-prompt.md"

// Decompose reads the decomposition prompt and the plan file at planFile, sends both to the
// provided LLM caller, and unmarshals the YAML response into a slice of TaskSpec.
//
// Returns the ordered slice of TaskSpec and an error if reading either file, calling the LLM,
// or unmarshalling fails.
func Decompose(planFile string, caller types.LLMCaller) ([]types.TaskSpec, error) {
	decompositionPrompt, err := os.ReadFile(decompositionPromptPath)
	if err != nil {
		return nil, fmt.Errorf("decompose: read decomposition prompt: %w", err)
	}

	planBody, err := os.ReadFile(planFile)
	if err != nil {
		return nil, fmt.Errorf("decompose: read plan file: %w", err)
	}

	prompt := string(decompositionPrompt) + "\n\n" + string(planBody)

	resp, err := caller.Call(context.Background(), prompt)
	if err != nil {
		return nil, fmt.Errorf("decompose: llm call: %w", err)
	}

	var tasks []types.TaskSpec
	if err := yaml.Unmarshal([]byte(resp), &tasks); err != nil {
		return nil, fmt.Errorf("decompose: unmarshal: %w", err)
	}
	return tasks, nil
}

// Dispatch assembles a prompt for the given task, calls the LLM, writes the output to a staging
// file, validates it inside an ephemeral container, and promotes the artifact on success.
// Validation failures and LLM errors are retried up to maxRetries times, accumulating prior error
// output into each subsequent prompt. Returns an error if all attempts fail or any non-retryable
// step errors.
func Dispatch(task types.TaskSpec, store types.ArtifactStore, caller types.LLMCaller, v types.Validator, maxRetries int) error {
	if len(task.ArtifactOut) == 0 {
		return fmt.Errorf("dispatch: no output artifact defined for task %s", task.ID)
	}
	ref := task.ArtifactOut[0].Ref

	// Staging directory: a temp dir per task, destroyed on exit.
	stagingDir, err := os.MkdirTemp("", "gordle-staging-"+task.ID+"-*")
	if err != nil {
		return fmt.Errorf("dispatch: create staging dir: %w", err)
	}
	defer os.RemoveAll(stagingDir)

	stagingFile := filepath.Join(stagingDir, filepath.Base(ref))

	grepIndex := map[string]string{} // populated by caller (T7/Run); empty is valid
	var priorErrors strings.Builder

	for attempt := 0; attempt <= maxRetries; attempt++ {
		prompt, err := injector.Assemble(task, store, grepIndex, priorErrors.String())
		if err != nil {
			return fmt.Errorf("dispatch: assemble: %w", err)
		}

		resp, err := caller.Call(context.Background(), prompt)
		if err != nil {
			fmt.Fprintf(&priorErrors, "\nllm call error (attempt %d): %v", attempt, err)
			continue
		}

		// Write output to staging path before validation so the container can mount and build it.
		if err := os.WriteFile(stagingFile, []byte(resp), 0o644); err != nil {
			return fmt.Errorf("dispatch: write staging file: %w", err)
		}

		// Validate inside ephemeral container; validator error = infrastructure failure, not retried.
		passed, output, err := v.Validate(task.ValidationCmd, stagingDir)
		if err != nil {
			return fmt.Errorf("dispatch: validator infrastructure error: %w", err)
		}
		if !passed {
			fmt.Fprintf(&priorErrors, "\nvalidation failed (attempt %d): %s", attempt, output)
			continue
		}

		// Derive next version: version 1 on first promote, incrementing on re-promotion.
		// Version 0 is never valid per artifact store layout.
		version := attempt + 1

		if err := store.Promote(ref, version, []byte(resp)); err != nil {
			fmt.Fprintf(&priorErrors, "\npromote error (attempt %d): %v", attempt, err)
			continue
		}

		return nil
	}

	return fmt.Errorf("dispatch: max retries exceeded for task %s:%s", task.ID, priorErrors.String())
}

// ResolveOrder performs a deterministic topological sort of tasks based on their DependsOn fields.
// Returns tasks in an order that satisfies all dependencies, or an error if a cycle is detected
// or a dependency references an unknown task ID.
//
// Exported so that main/Run can sequence the dispatch loop.
func ResolveOrder(tasks []types.TaskSpec) ([]types.TaskSpec, error) {
	idToTask := make(map[string]types.TaskSpec, len(tasks))
	for _, t := range tasks {
		idToTask[t.ID] = t
	}

	adj := make(map[string][]string)
	indeg := make(map[string]int)
	for _, t := range tasks {
		indeg[t.ID] = 0
	}
	for _, t := range tasks {
		for _, dep := range t.DependsOn {
			if _, ok := idToTask[dep]; !ok {
				return nil, fmt.Errorf("resolveorder: unknown dependency %q in task %s", dep, t.ID)
			}
			adj[dep] = append(adj[dep], t.ID)
			indeg[t.ID]++
		}
	}

	// Seed with zero-indegree nodes sorted for deterministic ordering.
	var ready []string
	for id, d := range indeg {
		if d == 0 {
			ready = append(ready, id)
		}
	}
	sort.Strings(ready)

	var ordered []types.TaskSpec
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		ordered = append(ordered, idToTask[id])

		// Sort newly unblocked dependents before appending for determinism.
		unblocked := make([]string, 0, len(adj[id]))
		for _, dep := range adj[id] {
			indeg[dep]--
			if indeg[dep] == 0 {
				unblocked = append(unblocked, dep)
			}
		}
		sort.Strings(unblocked)
		ready = append(ready, unblocked...)
	}

	if len(ordered) != len(tasks) {
		return nil, fmt.Errorf("resolveorder: dependency cycle detected")
	}
	return ordered, nil
}
