PIPELINE

# ----- PLAN HEADER

meta:
  title: "Go LLM Coding Harness"
  version: "v0.1.0"
  author: "ArghyV feat. Claude"
  date: "2026-05-18"

go:
  module: "github.com/ArghyV/gordle"
  packages:
    - main
    - queue
    - artifacts
    - llm
    - injector
    # prompt folded into injector (YAGNI — no separate package justified yet)
    - validator
  min_go_version: "1.22"
  validation_cmd: "go build ./... && go vet ./... && go test ./..."

artifacts:
  design_doc: "plan.md"
  task_specs_dir: ".harness/task_specs"
  code_output_dir: ".harness/artifacts"

dependencies:
  internal: []
  external:
    - "github.com/docker/docker"
    # Docker SDK for ephemeral validator containers

interfaces:
  - name: LLMCaller
    package: llm
    signatures:
      - "Call(ctx context.Context, prompt string) (string, error)"
  - name: ArtifactStore
    package: artifacts
    signatures:
      - "Promote(ref string, version int, content []byte) error"
      - "Get(ref string, version int) ([]byte, error)"
  - name: Validator
    package: validator
    signatures:
      - "Validate(cmd string, workdir string) (passed bool, output string, err error)"

error_convention: "All functions return (T, error). No panics. No sentinel errors."

entry_points:
  - signature: "Run(planFile string, opts Options) error"
    package: main
    # Top-level pipeline executor. Options carries --yolo, --max-retries, env.
  - signature: "Decompose(planFile string, caller LLMCaller) ([]TaskSpec, error)"
    package: queue
    # Calls decomposition LLM, parses ordered task spec list.
  - signature: "Dispatch(task TaskSpec, store ArtifactStore, caller LLMCaller, v Validator) error"
    package: queue
    # Injects context, calls LLM, validates, promotes or retries.
  - signature: "Assemble(task TaskSpec, store ArtifactStore, grepIndex map[string]string, priorErrors string) (string, error)"
    package: injector
    # Builds the full prompt string for a task: spec + artifact contents + grep index + prior error output.
    # Prompt template logic lives here (prompt package folded in, YAGNI).

constraints:
  no_circular_deps: true
  no_dot_imports: true
  no_multiline_string_literals: true
  explicit_type_signatures: true
  custom:
    - "Validator must run inside ephemeral container; container destroyed after each run"
    - "LLM API key loaded from .env in working dir if present; .env must be .gitignored"
    - "Cache keyed on SHA-256 of prompt string; cache stored in .harness/cache/"
    - "Max retry count: 2 on validation failure per task"

llm:
  provider_primary: "cerebras"
  provider_fallback: "none"
  # Fallback disabled (YAGNI). Add on first quota hit.
  model: "gpt-oss-120b"
  cache_deterministic_outputs: true

ordering_hints:
  - "T1: types/interfaces (LLMCaller, ArtifactStore, Validator, TaskSpec)"
  - "T2: artifacts package (flat store, versioned promotion)"
  - "T3: llm package (Cerebras caller, .env loader, prompt cache)"
  - "T4: validator package (ephemeral container runner)"
  - "T5: injector package (context assembly + prompt templates: spec + grep index + prior errors)"
  - "T6: queue package (dependency resolution, dispatch loop, retry handler)"
  - "T7: main (CLI flags: --yolo, --max-retries, --plan; confirmation prompts; activity indicator)"
  - "T8: integration tests"

-----

# PLAN BODY

## Overview

A Linux terminal harness that drives a Go code generation pipeline.
Takes a plan file as input, decomposes it into atomic task specs via an LLM,
dispatches tasks in dependency order, validates each output in an ephemeral container,
and promotes passing artifacts to a versioned store.

## Core Loop

1. Parse and validate plan header (decomposition-prompt.md rules).
2. Call LLM to decompose plan body → ordered []TaskSpec.
3. For each task (respecting depends_on edges):
   a. Assemble context: task_spec + artifact_in contents + grep index + prior error output.
   b. If --yolo=false (default): print prompt summary, ask user to confirm (y/n).
   c. Show activity indicator (spinner) while waiting for LLM response.
   d. Call LLM (Cerebras). On quota error: rotate provider per fallback list.
   e. Write output to staging path.
   f. Run validation_cmd inside ephemeral container (Docker or Podman).
   g. If pass: promote to artifact store at next version. Destroy container.
   h. If fail: append full validator stdout+stderr to context. Retry up to max_retries.
   i. If retries exhausted: halt, print task ID + final error, exit non-zero.

## Security Boundary

- LLM call: plain HTTPS to provider API. Not containerized.
- Validator: runs untrusted generated code. Runs in ephemeral container.
  Container image: golang:1.22-alpine. Destroyed after each run.
- No network access inside validator container.

## API Key Handling

- Load from .env file in working directory if present.
- On first run, if .env found and .gitignore does not contain .env entry: append it automatically.
- Env var name: CEREBRAS_API_KEY (and fallback provider keys as added).
- Never log key values.

## Artifact Store Layout

  .harness/
    artifacts/
      design_doc/   plan_v1.md
      task_specs/   T1_v1.yaml, T2_v1.yaml ...
      code_output/  token_struct_v1.go, ...
    cache/          {sha256_of_prompt}.json
    task_specs/     (working dir for decomposed specs)

## CLI Interface

  harness --plan plan.md [--yolo] [--max-retries 2]

  Flags:
    --plan          Path to plan file (required)
    --yolo          Disable per-call confirmation prompts
    --max-retries   Retry count on validation failure (default: 2)

## Activity Indicator

Spinner on stderr during any blocking LLM call.
Format: "⠋ Calling LLM [T2: implement issue()]..."
Cleared on response or error.

## Confirmation Prompt (default mode)

Before each LLM call, print:
  Task: T2 — implement issue()
  Provider: cerebras / llama-3.3-70b
  Artifacts in: [token_struct.go@v1]
  Proceed? [y/N]

N aborts current task (not full pipeline); user can resume.

## Out of Scope (YAGNI)

- Web UI
- Parallel task execution
- Semantic search / embeddings
- Multi-user / auth
- Artifact diffing
- Notifications
