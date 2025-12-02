# goagentbench

Benchmark AI coding agents on Go coding tasks. LLM/Agent pairs are considered when appropriate.

## testdata and results

Scenarios live in `testdata`. Each scenario has a `scenario.yml` file. No scenarios can be nested in other scenarios, scenarios can be nested in other non-scenario directories.

Raw results from running an agent on a scenario live in `results`.

## Constraints and limitations

This benchmark is not designed to handle writing **all** Go programs. For the sake of uniformity and simplicity:
- All programs are run in Linux.
- All programs must be runnable and verifyable with normal Go tests, or similar headless methods.
    - No programs may use browsers, GUIs, or similar to verify correctness.

## Guidelines

- Agents should be minimally configured. They should be close to clean installs.
- AGENTS.md in a repository root is allowed. Any agent my read it.
- The instructions given to each agent should be nearly identical.

Special Features

- An agent my offer "special features" besides just, "do this thing I type in the chat box". For instance, it may have a planning mode, reviewing mode, and so on.
- This presents a unique challenge to agent evaluation. We want to **encourage** agents to innovate. At the same time, we want to compare apples-to-apples.
- TODO: what do

## Scenario Classifications

The ontology is that a scenario belongs to a single `type`:
- `build-package`: build a new package from scratch.
- `fix-bug`: fix a bug in one or more packages.
- `add-feature`: add a new feature/enhancement in one or more packages.

Beyond it's overall type, a scenario may have zero or more `properties`:
- `has-spec`: true or false. The task has a SPEC.md or similar.
- `single-package`: true or false. The task is isolated to a single package vs spans multiple packages.
- `sees-failing-tests`: true or false. If true, the failing tests are provided that the agent is measured against. If false, the agent is evaluated on tests it cannot see.