// Package injector assembles prompts for language model calls by combining task specifications,
// artifact contents, grep index entries, and prior error information. It does not perform any
// I/O beyond retrieving artifacts from the provided store.
package injector

import (
	"fmt"
	"strings"

	"github.com/ArghyV/gordle"
	"gopkg.in/yaml.v3"
)

// Assemble builds a prompt string for the given task.
//
// It marshals the task specification to YAML, retrieves each input artifact from the
// provided store, appends the grep index entries, and includes any prior error messages.
// The function returns the assembled prompt or an error if any step fails.
func Assemble(task gordle.TaskSpec, store gordle.ArtifactStore, grepIndex map[string]string, priorErrors string) (string, error) {
	var sb strings.Builder

	// Marshal the task specification to YAML.
	taskYAML, err := yaml.Marshal(task)
	if err != nil {
		return "", fmt.Errorf("assemble: failed to marshal task spec: %w", err)
	}
	sb.WriteString("TaskSpec:\n")
	sb.Write(taskYAML)
	sb.WriteString("\n")

	// Append each input artifact's content.
	if len(task.ArtifactIn) > 0 {
		sb.WriteString("ArtifactIn:\n")
		for _, ref := range task.ArtifactIn {
			// Retrieve the artifact content; version 0 is used as a default.
			content, getErr := store.Get(ref.Ref, 0)
			if getErr != nil {
				return "", fmt.Errorf("assemble: failed to get artifact %s: %w", ref.Ref, getErr)
			}
			sb.WriteString(fmt.Sprintf("- Ref: %s\n  Content: |\n", ref.Ref))
			// Indent content lines for readability.
			for _, line := range strings.Split(string(content), "\n") {
				sb.WriteString("    " + line + "\n")
			}
		}
		sb.WriteString("\n")
	}

	// Append grep index entries.
	if len(grepIndex) > 0 {
		sb.WriteString("GrepIndex:\n")
		for k, v := range grepIndex {
			sb.WriteString(fmt.Sprintf("- %s: %s\n", k, v))
		}
		sb.WriteString("\n")
	}

	// Append prior errors block.
	if priorErrors != "" {
		sb.WriteString("PriorErrors:\n")
		for _, line := range strings.Split(priorErrors, "\n") {
			sb.WriteString("  " + line + "\n")
		}
		sb.WriteString("\n")
	}

	return sb.String(), nil
}
