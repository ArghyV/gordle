---
# PIPELINE PLAN HEADER
# Prepend this block to any freeform plan file.
# All fields required unless marked OPTIONAL.

meta:
  title: ""                        # Feature or component name
  version: ""                      # e.g. v0.1.0
  author: ""
  date: ""                         # ISO 8601

go:
  module: ""                       # e.g. github.com/org/repo
  packages:                        # All packages this plan touches
    - ""
  min_go_version: ""               # e.g. 1.22
  validation_cmd: "go build ./... && go vet ./... && go test ./..."

artifacts:
  design_doc: ""                   # Path or ref to this plan file
  task_specs_dir: ""               # Where decomposed task specs will be written
  code_output_dir: ""              # Target source directory

dependencies:
  internal:                        # Packages within module this plan depends on
    - ""
  external:                        # Third-party imports required
    - ""

interfaces:
  # Explicit contracts this plan must satisfy or implement.
  # Format: InterfaceName → package where defined
  - name: ""
    package: ""

error_convention: ""
  # e.g. "All functions return (T, error). No panics. No sentinel errors."

entry_points:
  # Public-facing functions/methods this plan exposes on completion.
  # Format: func Signature → package
  - signature: ""
    package: ""

constraints:
  no_circular_deps: true
  no_dot_imports: true
  no_multiline_string_literals: true
  explicit_type_signatures: true
  custom:                          # OPTIONAL: additional project-specific rules
    - ""

llm:
  provider_primary: ""             # e.g. google-ai-studio / anthropic
  provider_fallback: ""            # e.g. groq / cerebras
  model: ""                        # e.g. gemini-2.5-flash / claude-sonnet-4-6
  cache_deterministic_outputs: true

ordering_hints:                    # OPTIONAL: known dependency order or phasing
  - ""
---
