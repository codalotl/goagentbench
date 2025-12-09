# goagentbench

Benchmark AI coding agents on Go coding tasks.

This benchmark aims to measure not only correctness, but the {correctness, cost, speed} tradeoff space.

## testdata and results

Scenarios live in `testdata`. Each scenario has a `scenario.yml` file. No scenarios can be nested in other scenarios, scenarios can be nested in other non-scenario directories.

Raw results from running an agent on a scenario live in `results`.

## Constraints and limitations

This benchmark is not designed to handle writing **all** Go programs. For the sake of uniformity and simplicity:
- All programs are run in Linux.
- All programs must be runnable and verifiable with normal Go tests, or similar headless methods.
    - No programs may use browsers, GUIs, or similar to verify correctness.

## Guidelines

- If a scenario needs to be modified after results have been published for it, append semver to it (the initial version is considered version 1). Ex: `myscenario-1.1`.
- Agents should be minimally configured. They should be close to clean installs.
- AGENTS.md in a repository root is allowed. Any agent may read it.
- The instructions given to each agent should be nearly identical.

## CLI

Build or run the CLI from the repository root:

Workspace directory defaults to `./workspace`; override with the `GOAGENTBENCH_WORKSPACE` environment variable.

- `goagentbench validate-scenario <scenario>`: validate `testdata/<scenario>/scenario.yml` (uses `git ls-remote` to confirm the commit exists).
- `goagentbench setup <scenario>`: clone the scenario repo at the requested commit into the workspace and apply setup copy steps.
- `goagentbench run-agent --agent=<agent> [--model=<model>] <scenario> [--only-start]`: create `.run-start.json` and optionally invoke the agent harness (manual by default for codalotl; codex uses the built-in harness that shells out to `codex exec`). If `--model` is omitted, the first `supports-llms` entry for the agent is used.
- `goagentbench verify <scenario> [--only-report]`: run verification tests, print a summary, and write a JSON report under `results/<scenario>/`.

Special Features

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
