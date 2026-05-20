// Package queue provides functions for decomposing a plan into tasks, resolving task execution order,
// and dispatching tasks with retry, validation, and artifact promotion.
//
// It does not implement any persistence, concurrency, or external orchestration beyond the provided interfaces.
package queue

import (
	"context"
	"fmt"
	"os"

	"github.com/ArghyV/gordle"
	"github.com/ArghyV/gordle/injector"
	"gopkg.in/yaml.v3"
)

// Decompose reads the plan file at planFile, sends a decomposition prompt together with the plan
// body to the provided LLM caller, and unmarshals the LLM response (YAML) into a slice of TaskSpec.
//
// Returns the slice of TaskSpec and an error if reading the file, calling the LLM, or unmarshalling fails.
func Decompose(planFile string, caller gordle.LLMCaller) ([]gordle.TaskSpec, error) {
	// Read the plan file.
	data, err := os.ReadFile(planFile)
	if err != nil {
		return nil, fmt.Errorf("decompose: read file error: %w", err)
	}

	// Build a simple prompt for the LLM.
	prompt := fmt.Sprintf("Decompose the following plan into tasks:\n%s", string(data))

	// Call the LLM.
	resp, err := caller.Call(context.Background(), prompt)
	if err != nil {
		return nil, fmt.Errorf("decompose: llm call error: %w", err)
	}

	// Unmarshal the YAML response into []TaskSpec.
	var tasks []gordle.TaskSpec
	if err := yaml.Unmarshal([]byte(resp), &tasks); err != nil {
		return nil, fmt.Errorf("decompose: unmarshal error: %w", err)
	}
	return tasks, nil
}

// Dispatch assembles a prompt for the given task, calls the LLM, validates the generated output,
// and promotes the artifact on success. If validation fails or the LLM call errors, the function
// retries up to maxRetries times, aggregating prior error messages. Returns an error if all
// attempts fail or if any internal step returns an error.
func Dispatch(task gordle.TaskSpec, store gordle.ArtifactStore, caller gordle.LLMCaller, v gordle.Validator, maxRetries int) error {
	var priorErrors string

	// Grep index is not provided by the current context; use an empty map.
	grepIndex := map[string]string{}

	// Attempt execution up to maxRetries+1 times (initial try + retries).
	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Assemble the prompt.
		prompt, err := injector.Assemble(task, store, grepIndex, priorErrors)
		if err != nil {
			return fmt.Errorf("dispatch: assemble error: %w", err)
		}

		// Call the LLM.
		resp, err := caller.Call(context.Background(), prompt)
		if err != nil {
			// Accumulate the error message and continue to next retry.
			priorErrors = fmt.Sprintf("%s\nllm call error: %v", priorErrors, err)
			continue
		}

		// Validate the generated response.
		passed, output, err := v.Validate(resp, ".")
		if err != nil {
			// Validation infrastructure error; treat as fatal.
			return fmt.Errorf("dispatch: validator error: %w", err)
		}
		if !passed {
			// Validation failed; accumulate output and retry.
			priorErrors = fmt.Sprintf("%s\nvalidation failed: %s", priorErrors, output)
			continue
		}

		// Promotion: use the first output artifact reference if defined, otherwise error.
		if len(task.ArtifactOut) == 0 {
			return fmt.Errorf("dispatch: no output artifact defined for task %s", task.ID)
		}
		ref := task.ArtifactOut[0].Ref
		if err := store.Promote(ref, 0, []byte(resp)); err != nil {
			// Promotion failed; accumulate error and retry.
			priorErrors = fmt.Sprintf("%s\npromote error: %v", priorErrors, err)
			continue
		}

		// Success.
		return nil
	}

	// Exhausted retries.
	return fmt.Errorf("dispatch: max retries exceeded:%s", priorErrors)
}

// resolveOrder performs a topological sort of the provided tasks based on their DependsOn fields.
// Returns a slice of tasks in an order that satisfies dependencies, or an error if a cycle is detected
// or if a dependency references an unknown task.
func resolveOrder(tasks []gordle.TaskSpec) ([]gordle.TaskSpec, error) {
	// Map task ID to its specification.
	idToTask := make(map[string]gordle.TaskSpec, len(tasks))
	for _, t := range tasks {
		idToTask[t.ID] = t
	}

	// Build adjacency list and indegree count.
	adj := make(map[string][]string) // key -> list of dependents
	indeg := make(map[string]int)
	for _, t := range tasks {
		indeg[t.ID] = 0
	}
	for _, t := range tasks {
		for _, dep := range t.DependsOn {
			if _, ok := idToTask[dep]; !ok {
				return nil, fmt.Errorf("resolveorder: unknown dependency %s", dep)
			}
			adj[dep] = append(adj[dep], t.ID)
			indeg[t.ID]++
		}
	}

	// Kahn's algorithm.
	var queue []string
	for id, d := range indeg {
		if d == 0 {
			queue = append(queue, id)
		}
	}
	var ordered []gordle.TaskSpec
	for len(queue) > 0 {
		// Pop first element.
		id := queue[0]
		queue = queue[1:]

		ordered = append(ordered, idToTask[id])

		for _, dep := range adj[id] {
			indeg[dep]--
			if indeg[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	if len(ordered) != len(tasks) {
		return nil, fmt.Errorf("resolveorder: dependency cycle detected")
	}
	return ordered, nil
}
