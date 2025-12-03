# goagentbench

This package is a main package, and the entrypoint of the CLI tool to orchestrate the running of agent benchmarks.

Read `README.md` for more background.

## CLI Usage

It is assumed that the `goagentbench` exists and is run from this repo's root dir. Users may also just do `go run .` instead of compiling the binary.

There is some path on the filesystem that is the workspace. Within this doc, it is referred to as `$WORKSPACE` or just "the workspace".  It is just `./workspace` unless overridden.

In the examples below, I am using `tui_build` as an example scenario. It can be replaced with any valid scenario. This directory may be referred to as `$SCENARIODIR`.

Running the scenario means checking out a repo to `$WORKSPACE/$SCENARIODIR` applying setup steps there, and then letting the agent run there.

### validate-scenario

`goagentbench validate-scenario tui_build`: validates the `scenario.yml` file is valid. Any files, data, repos, and commits referenced in the `scenario.yml` file exist. It will either print out "valid" or print out any problems.

### setup

`goagentbench setup tui_build`: sets up the source tree for this scenario within the workspace (fetches repo, checks out sha, applies setup steps in the scenario). `tui_build` must exist `testdata`. This parameter may have slashes to navigate to a nested subdirectory in `testdata`. If setup was already run on this scenario (possibly with agent runs dirtying it), setup provide a clean setup of `tui_build`.

### run-agent

`goagentbench run-agent --agent=codex --model=gpt-5-codex-high tui_build`: runs the agent on the scenario.

Validations:
- `--agent` is required. `--model` is optional. Both are valid.
- `tui_build` exists as a scenario in the workspace. No existing run exists for this directory.

It first writes run metadata in `$WORKSPACE/tui_buid/.run-start.json`. This data includes a run ID, the start time and date, the agent, the model, and some system information (ex: OS).

Then it will run the agent against the scenario. As the agent takes turns, it will keep up to date `$WORKSPACE/tui_buid/.run-progress.json` (even if multiple prompts are taken). This file contains token usage, execution time, end time, and agent transcripts.

If the `--only-start` option is used, only the `.run-start.json` file is created. The agent can then be manually run, recording things like token usage and execution time manually (or with other tools/subcommands).

### verify

`goagentbench verify tui_build`: verifies the agent's progress against the scenario by executing the verification steps.

By default, it writes a verification report to `./results` in:
`results/tui_build/yyyy-mm-dd-<run_id>-<agent>-<model>.verify.json`
(example: results/tui_build/2025-12-03-run_1234567890-codex-gpt-5-codex-high.verify.json).

It also prints out a summary of the report. The report includes both summary/numerical information (e.g., "agent got 5 of 8 tests to pass"), as well as logs of the verification process.

## scenario.yml

Below is an example yml file.

Required keys:
- `name`, `repo`, `commit`, `classification` (`type` subfield required), `setup`, `agent` (`instructions` subfield required), `verify`

```yml
# name is a basic < 1 sentence description of the scenario.
name: Building a TUI framework package from a spec

# repo: repository we're operating on
repo: github.com/codalotl/codalotl

# commit: which SHA to checkout to do the test.
commit: 70744dc5b999bce4d0ac82329b2cd7e2bfb2c252

# classification: which type and properties the scenario is classified as.
# this lets us slice and dice the results to see where agents shine.
classification:
  type: build-package
  has-spec: true
  single-package: true
  sees-failing-tests: true

# setup: how to prepare the filesystem after checkout of repo/sha.
# - `setup: null` is possible if the sha gives a ready-to-go scenario.
# - Alternatively, we may want to copy over failing test cases, apply a patch, etc.
setup:
  # copy: an array of from/to pairs.
  copy:
    - from: some_test.go # relative to scenario directory in `testdata`
      to: path/to/package # relative to $WORKSPACE/$SCENARIODIR. Can be ".".
  
  # FUTURE: we could do patches: array of patches. Could also do scripts: array of scripts.

# Instructions and other agent configuration for this problem.
# This COULD involve per-agent specialization?

# agent: instructions and configuration we give to ALL agents.
agent:
  # instructions: prompt we tell the agent.
  instructions: |
    In internal/q/tui, read the SPEC.md and build the package according to the spec.
    Do not install or use third party packages that aren't already used in go.mod.
    Do not modify the SPEC.md or any provided tests. You may write new tests.
  
  # allow-multiple-turns: if the agent ends its turn before solving the problem, this allows it to continue.
  # When being prompted to continue, we will just send, "Please continue until the problem is solved."
  # We ask the agent to continue IF verify does not pass AND this option is true.
  # We limit usage of this to 3 continues (may be configurable in future).
  allow-multiple-turns: true

  # FUTURE IDEAS:
  # plan: true # let planning agents actually do their /plan feature. Non-planning agents are told a generic "make a plan" instruction.
    

# How do we know if the agent succeeded?
# - Existing tests pass (need to ensure the agent didn't delete or disable the tests, which they sometimes do!)
# - Copy test files(s) from this directory to some path and run them.
# - Ensure snippet signatures exist as we expect
# - Ensure the copied test files type check correctly. This could be a better way to verify snippet signatures.
# - Ensure some files are not modified. Ex: only files in package are modified; no new modules installed.

# verify: how we know if the agent succeeded in this scenario.
# There are two levels of success: complete success and partial success. Partial success is a number from 0 to 1 (ex: 0.84). Complete success is a 1.
# For complete success, all checks and tests listed must pass (tests not mentioned need not pass).
# For partial success:
# - Partial success is only possible if explicitly enabled via `partial-success: true` (not all scenarios really lend themselves to partial success).
# - If everything passes EXCEPT for tests, 
verify:
   
  only-modify: internal/q/tui # only-modify-recursive could also exist
  no-modify:
    - internal/q/tui/golden* # Do not modify any golden test files
    - internal/q/tui/SPEC.md # Do not modify the spec.
  tests: "go test ./internal/q/tui"
```