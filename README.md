# goagentbench

Benchmark AI coding agents and LLMs on Go-only coding tasks.

- This benchmark aims to measure not only correctness, but the {correctness, cost, speed} trade-off space.
- We measure specific pairs of agents+LLMs, because agents matter and are often optimized for certain LLMs.
- The scenarios we benchmark are **real-world** scenarios in **actual Go repos**, not artificial algorithmic stuff like "implement XYZ algorithm".
- For most scenarios, the agent does not see failing test cases, like they do in many benchmarks. This makes it harder, but more realistic.

## Results

TODO: add current dated results

## Concepts and Repo Structure

A benchmark "test" in goagentbench is called a _scenario_. Each scenario is a self-contained task definition rooted at a directory under `testdata/`, with its main entry point at `scenario.yml`.

At a high level, running a scenario means:

- **Scenario definition**: `testdata/<scenario>/scenario.yml` points at a real Go repo+commit and defines setup steps, agent instructions, and verification rules.
- **Workspace**: the repo is checked out and prepared in `workspace/<scenario>/` (override with `GOAGENTBENCH_WORKSPACE`).
- **Agent run metadata**: `run-agent` records run info in `workspace/<scenario>/.run-start.json` and updates `workspace/<scenario>/.run-progress.json` as the agent runs.
- **Verification + results**: `verify` enforces modification rules, runs the scenario's `go test` targets, and writes a report under `results/<scenario>/` (override with `GOAGENTBENCH_RESULTS`).

Most of the orchestrator implementation lives in `internal/` (CLI commands, scenario parsing/validation, setup, agent harnesses, and verification).

## Instructions

### Running Scenarios

The recommended way to run scenarios is inside the dev container:

- Start a shell: `./docker_dev.sh`
- Run an end-to-end scenario: `go run . exec --agent=codex --model=gpt-5.2-high self/copy_to_dir`

The container sets `GOAGENTBENCH_RESULTS=/host/results`, so verification reports written inside the container show up on your host in `results/`.

That being said, all commands work directly on, for instance, an OSX laptop.

Common subcommands (instead of `exec`) are:
- `go run . validate-scenario <scenario>`
- `go run . setup <scenario>`
- `go run . run-agent --agent=<agent> [--model=<model>] <scenario>`
- `go run . verify <scenario>`

Useful environment variables:
- `GOAGENTBENCH_WORKSPACE`: override `workspace/`
- `GOAGENTBENCH_RESULTS`: override `results/`
- `GOAGENTBENCH_SCENARIO_ROOT`: override `testdata/`
- `GOAGENTBENCH_SKIP_REMOTE`: skip `git ls-remote` commit checks

### Adding Scenarios

- Create a new folder under `testdata/` containing a `scenario.yml` (nest by repo, e.g. `testdata/<repo>/<scenario>/scenario.yml`).
- Point the scenario at a specific repo+commit, and define `setup`, `agent.instructions` (the prompt), and `verify` (modification rules + `go test` targets).
- Prefer verification via Go tests. If you need hidden tests, use `verify.copy` to copy them into the workspace during verification.
- Avoid clobbering tests the agent might create. A good pattern is to copy integration tests into a dedicated, test-only package.
- Expect iteration: run it across multiple agents/models and tighten the prompt + tests until the task is unambiguous.

Hard requirements:
- Must run on Linux (the `./docker_dev.sh` container is the reference environment).
- Must be verifiable with `go test` (no browsers/GUI-driven verification; no custom test scripts).
- If you need to change a scenario after publishing results, create a new version by appending a semver suffix (e.g. `myscenario-1.1`).

### Adding Agents/LLMs

To add a new model configuration:
- Add an entry to `llms.yml` (the `name` is what you pass via `--model`).
- Use `per-agent` when different agents need different model identifiers for the same underlying model.

To add a new agent:
- Implement the harness in `internal/agents/` (run it, extract transcripts/token usage, and report its version).
- Add the pinned agent `version` and its `supports-llms` list to `agents.yml`.
- Keep `Dockerfile` installs and `agents.yml` versions in sync.

Agent requirements:
- The agent MUST implement a CLI mode (noninteractive. No GUI; no TUI).
- The agent SHOULD report token usage and cost (otherwise, you can manually enter it).
- The agent SHOULD support session resumes.

### Agent Guidelines

- Agents should be minimally configured. They should be close to clean installs.
- AGENTS.md in a repository root is allowed. Any agent may read it.
- The instructions given to each agent should be nearly identical.
- An agent may offer "special features" besides just, "do this thing I type in the chat box". For instance, it may have a planning mode, reviewing mode, and so on.
- This presents a unique challenge to agent evaluation. We want to **encourage** agents to innovate. At the same time, we want to compare apples-to-apples.
- Special features will need to be considered on a case-by-case basis, and possibly configured/classified in the scenario.

## Scenario Classifications

The ontology is that a scenario belongs to a single `type`:
- `build-package`: build a new package from scratch.
- `fix-bug`: fix a bug in one or more packages. Fixing a bug also often involves refining a feature, and potentially refactoring.
- `feature`: add a new feature/enhancement in one or more packages. This also includes, "continue development".
- `refactor`: no semantic changes expected.

Note that the above ontology is not an exact match to the real world. That's okay. This is just to slice and dice metrics for better analysis.

Beyond its overall type, a scenario may have zero or more `properties`:
- `has-spec`: true or false. The task has a SPEC.md or similar.
- `single-package`: true or false. The task is isolated to a single package vs spans multiple packages.
- `sees-failing-tests`: true or false. If true, the failing tests are provided that the agent is measured against. If false, the agent is evaluated on tests it cannot see (or not applicable).
