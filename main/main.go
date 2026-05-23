// Package main provides the CLI entry point for the gordle system. It does not implement any core logic beyond orchestration.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ArghyV/gordle/types"
	"github.com/ArghyV/gordle/artifacts"
	"github.com/ArghyV/gordle/llm"
	"github.com/ArghyV/gordle/queue"
	"github.com/ArghyV/gordle/validator"
)

// Run orchestrates the decomposition and dispatch of tasks defined in the plan file.
// It loads the API key, ensures a .gitignore entry, creates required components, and
// processes each task with optional confirmation and retry handling.
// Returns an error if any step fails.
func Run(planFile string, opts types.Options) error {
	// Load the Cerebras API key from the .env file.
	apiKey, err := llm.LoadAPIKey(".env")
	if err != nil {
		return fmt.Errorf("run: load api key error: %w", err)
	}

	// Ensure .gitignore contains an entry for .env.
	if err := llm.EnsureGitignore("."); err != nil {
		return fmt.Errorf("run: ensure gitignore error: %w", err)
	}

	// Create a cache for LLM responses.
	cache := llm.NewCache(".cache")

	// Read the worker system prompt once; used only for dispatch (code generation) calls.
	// The decomposition caller intentionally omits a system prompt to avoid conflicting
	// instructions with the decomposition prompt's own output format requirements.
	workerSystemPrompt, err := os.ReadFile("worker-system-prompt.md")
	if err != nil {
		return fmt.Errorf("run: read worker system prompt: %w", err)
	}

	const cerebrasModel = "gpt-oss-120b"
	decomposeCaller := llm.NewCerebras(apiKey, cerebrasModel, "", cache)
	workerCaller := llm.NewCerebras(apiKey, cerebrasModel, string(workerSystemPrompt), cache)

	// Create the artifact store.
	store := artifacts.NewStore(".artifacts")

	// Create the validator.
	validator := validator.NewContainerValidator("golang:1.22-alpine")

	// Decompose the plan into tasks.
	tasks, err := queue.Decompose(planFile, decomposeCaller)
	if err != nil {
		return fmt.Errorf("run: decompose error: %w", err)
	}

	// Process each task in order.
	for _, task := range tasks {
		// Prompt for confirmation unless yolo mode is enabled.
		if !opts.YoloMode {
			confirmed, err := confirmTask(task)
			if err != nil {
				return fmt.Errorf("run: confirm task error: %w", err)
			}
			if !confirmed {
				fmt.Fprintf(os.Stdout, "skipping task %s\n", task.ID)
				continue
			}
		}

		// Start a spinner while dispatching the task.
		stopSpinner := spinnerStart(fmt.Sprintf("dispatching task %s", task.ID))
		err = queue.Dispatch(task, store, workerCaller, validator, opts.MaxRetries)
		stopSpinner()
		if err != nil {
			return fmt.Errorf("run: dispatch error: %w", err)
		}
	}

	return nil
}

// main parses command‑line flags, validates required inputs, and invokes Run.
// It exits with a non‑zero status code if an error occurs.
func main() {
	var plan string
	var yolo bool
	var maxRetries int

	flag.StringVar(&plan, "plan", "", "path to the plan file (required)")
	flag.BoolVar(&yolo, "yolo", false, "enable yolo mode to skip confirmations")
	flag.IntVar(&maxRetries, "max-retries", 2, "maximum number of retry attempts for each task")
	flag.Parse()

	if plan == "" {
		fmt.Fprintln(os.Stderr, "error: --plan flag is required")
		os.Exit(1)
	}

	opts := types.Options{
		YoloMode:   yolo,
		MaxRetries: maxRetries,
		PlanFile:   plan,
	}

	if err := Run(plan, opts); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// confirmTask displays task details and prompts the user for confirmation.
// It returns true if the user confirms, false if the user declines, and an error on I/O failure.
func confirmTask(task types.TaskSpec) (bool, error) {
	fmt.Printf("Task ID: %s\n", task.ID)
	fmt.Printf("Package: %s\n", task.Package)
	fmt.Printf("ArtifactIn:\n")
	if len(task.ArtifactIn) == 0 {
		fmt.Println("  (none)")
	} else {
		for _, a := range task.ArtifactIn {
			fmt.Printf("  - %s (%s)\n", a.Ref, a.Type)
		}
	}
	fmt.Print("Proceed? (y/n): ")

	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("confirmtask: read input error: %w", err)
	}
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes", nil
}

// spinnerStart launches a simple spinner that writes to stderr with the given label.
// It returns a function that stops the spinner when called. The caller should defer the
// returned function to ensure the spinner is stopped.
func spinnerStart(label string) func() {
	done := make(chan struct{})
	go func() {
		chars := []rune{'|', '/', '-', '\\'}
		i := 0
		for {
			select {
			case <-done:
				fmt.Fprint(os.Stderr, "\r")
				return
			default:
				fmt.Fprintf(os.Stderr, "\r%s %c", label, chars[i%len(chars)])
				i++
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()
	return func() {
		close(done)
		fmt.Fprint(os.Stderr, "\r")
	}
}
