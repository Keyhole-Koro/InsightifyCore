# Insightify Command Reference

This document outlines the CLI tools and the internal processing pipeline of InsightifyCore.

## CLI Tools

Entry points located in the `cmd/` directory.

### `archflow`

The primary analysis orchestration tool. It runs the pipeline steps defined in the graph.

- **Usage**: `go run ./cmd/archflow --repo <path> --phase <phase> [options]`
- **Key Flags**:
  - `--repo`: Path to the target repository to analyze.
  - `--phase`: The analysis phase to execute (e.g., `c1`).
  - `--provider`: LLM provider to use (e.g., `gemini`).
  - `--model`: Specific model to use (e.g., `gemini-2.5-pro`).
- **Phases**:
  - `c`: Codebase
  - `a`: Algorithm
  - `x`: External
  - `b`: Build
  - `m`: Architecture

#### Examples

Run the codebase analysis phase on the current directory:

```bash
go run ./cmd/archflow --repo . --phase c
```

Run the architecture analysis phase using a specific Gemini model:

```bash
go run ./cmd/archflow --repo /path/to/repo --phase a --provider gemini --model gemini-1.5-pro
```

### `viz`

A tool for visualizing the analysis outputs.

- **Purpose**: Renders Mermaid graphs or serves a UI to explore the generated insights.

#### Examples

Start the visualization server to view results:

```bash
go run ./cmd/archflow --viz > graph.mermaid
```

## Internal Pipeline Steps

## 1. Initialization & Analysis

This phase involves understanding the repository structure and identifying languages and configurations.

### `code_roots`

- **Summary**: Repository structure scan and root classification.
- **Details**: Scans the repository layout and asks the LLM to classify "main source roots", "library/vendor roots", and "config hotspots".
- **Dependencies**: None (Entry point)

### `code_specs`

- **Summary**: Identification of language specifications and import rules.
- **Details**: Based on file extension distribution and `code_roots`, the LLM infers the project's language families and import heuristics.
- **Dependencies**: `code_roots`

## 2. Dependency Graph Construction

This phase analyzes code dependencies, splits them into processable tasks, and extracts symbol information.

### `code_imports`

- **Summary**: Dependency sweeping.
- **Details**: Performs a word-index based dependency sweep across source roots to collect possible file-level dependencies.
- **Dependencies**: `code_roots`, `code_specs`

### `code_graph`

- **Summary**: Dependency graph normalization.
- **Details**: Normalizes detected dependencies into a DAG (Directed Acyclic Graph) and drops weaker bidirectional edges to reduce noise.
- **Dependencies**: `code_imports`

### `code_tasks`

- **Summary**: Splitting into LLM tasks.
- **Details**: Chunks graph nodes into tasks sized for LLM processing, using token estimates per file.
- **Dependencies**: `code_graph`

### `code_symbols`

- **Summary**: Building symbol reference maps.
- **Details**: The LLM traverses the tasks to build maps of identifier references (outgoing/incoming).
- **Dependencies**: `code_tasks`

## 3. Architecture & Infrastructure Inference

This phase infers the overall system architecture and infrastructure configuration by combining code analysis results and documentation.

### `arch_design`

- **Summary**: Drafting architecture hypotheses.
- **Details**: The LLM drafts an initial architecture hypothesis based on the file index and Markdown documents, proposing the next files to investigate.
- **Dependencies**: `code_roots`

### `infra_context`

- **Summary**: Summarizing infrastructure/external systems.
- **Details**: Combines `arch_design` (architecture hypothesis) and `code_symbols` (identifier refs) for the LLM to summarize external systems and infrastructure configurations, surfacing evidence gaps.
- **Dependencies**: `arch_design`, `code_symbols`, `code_roots`

### `infra_refine`

- **Summary**: Deep dive into infrastructure.
- **Details**: The LLM drills into the evidence gaps identified in `infra_context` by opening targeted files or snippets to refine the information.
- **Dependencies**: `infra_context`
