# goagentbench

This package is a main package, and the entrypoint of the CLI tool to orchestrate the running of agent benchmarks.

Read `README.md` for more background.

## CLI Usage

It is assumed that the `goagentbench` exists and is run from this repo's root dir. Users may also just do `go run .` instead of compiling the binary.

There is some path on the filesystem that is the workspace. Within this doc, it is referred to as `$WORKSPACE` or just "the workspace".  It is just `./workspace` unless overridden. It can be overridden by the env var `$GOAGENTBENCH_WORKSPACE`.

In the examples below, I am using `tui_build` as an example scenario. It can be replaced with any valid scenario. This directory may be referred to as `$SCENARIODIR`.

Running the scenario means checking out a repo to `$WORKSPACE/$SCENARIODIR` applying setup steps there, and then letting the agent run there.

Results are written to `./results` (see ## verify below) by default. This directory can be overriden by the env var `$GOAGENTBENCH_RESULTS`.

## Output

Since this is a CLI app, we output to a terminal. But this CLI app is an **orchestrator** -- it often execs other commands, and those commands also have output. As such, it can be difficult to determine which output came from where.

Output from **cobra** or "pre-run" stuff can remain as is, and this doesn't apply. For instance, printing out usage, or failing to supply a required flag, shouldn't be in bold.

All output from **this** app (fmt.Println, for instance) will be output in Bold (using ANSI CSI sequences).

All output from exec'ed commands (ex: `git clone`; output from agents; `go test`; etc) will be non-bold and italic, and will be colorized to a particular color in the 256-color space (which one depends on dark vs light mode). Any other formatting from exec'ed commands will be removed (ex: if command wrote underline sequence, we will remove that). Unless otherwise noted, all commands will echo this reformatted output to the temrinal.

If any command is run (ex: `git clone`), the full comamnd will be printed first in non-bold, non-italic, and colorized to a different color than above.

Finally, there MUST be a blank line between OUR output and THEIR output (no blank line between command run and its output). This in a blank line AFTER their output. Example (using tags to indicate ANSI formatting for ease of human reading):

```
<b>Running agent now.<reset>

<color1>codex exec --dangerously-bypass-approvals-and-sandbox --skip-git-repo-check --json "some instructions"<reset>
<i><color2>Codex: i'm starting work...<reset>
...
<i><color2>Codex: finished!<reset>

<b>Finished running agent. Starting verification...<reset>
```

### validate-scenario

`goagentbench validate-scenario tui_build`: validates the `scenario.yml` file is valid. Any files, data, repos, and commits referenced in the `scenario.yml` file exist. It will either print out "valid" or print out any problems.

### setup

`goagentbench setup tui_build`: sets up the source tree for this scenario within the workspace (fetches repo, checks out sha, applies setup steps in the scenario). `tui_build` must exist in `testdata`. This parameter may have slashes to navigate to a nested subdirectory in `testdata`. If setup was already run on this scenario (possibly with agent runs dirtying it), setup provides a clean setup of `tui_build`.

### run-agent

`goagentbench run-agent --agent=codex --model=gpt-5-codex-high tui_build`: runs the agent on the scenario.

Validations:
- `--agent` is required. `--model` is optional. If omitted, it defaults to the first entry in the agent's `supports-llms` list (as long as that model exists in `llms.yml`).
- `tui_build` exists as a scenario in the workspace. No existing run exists for this directory.

It first writes run metadata in `$WORKSPACE/tui_build/.run-start.json`. This data includes a run ID, the start time and date, the agent and version, the model, and some system information (ex: OS).

Then it will run the agent against the scenario. As the agent takes turns, it will keep up to date `$WORKSPACE/tui_build/.run-progress.json` (even if multiple prompts are taken). This file contains token usage, execution time, end time, and agent transcripts.

If the `--only-start` option is used, only the `.run-start.json` file is created. The agent can then be manually run, recording things like token usage and execution time manually (or with other tools/subcommands).

### verify

`goagentbench verify tui_build`: verifies the agent's progress against the scenario by executing the verification steps.

By default, it writes a verification report to `./results` in:
`results/tui_build/yyyy-mm-dd-<run_id>-<agent>-<model>.verify.json`
(example: `results/tui_build/2025-12-03-run_1234567890-codex-gpt-5-codex-high.verify.json`).

When it does this, it combines the data in `.run-start.json` and `.run-progress.json`, as well as verification info, to write the final `verify.json` file.

It also prints out a summary of the report. The printed summary includes which verification steps passed and failed. In partial success cases, it prints out the fraction of passing tests.

If the `--only-report` option is used, it only prints out the summary report, and does not write a file to `./results`. When used with this option, `verify` should be able to be used with or without a `.run-progress.json` file.

Calling `verify` with or without the `--only-report` flag should be idempotent.

## exec

`goagentbench exec --agent=codex [--model=gpt-5.1-codex-max-medium] tui_build`:
- Runs `validate-scenario`
- Runs `setup`
- Runs `run-agent`
- Runs `verify`
- If any steps fail, we abort the pipeline (ex: validate scenario found issue; setup cannot clone repo; agent exits with status 1).

## scenario.yml

Below is an example yml file with field descriptions, semantic meaning, and rules.

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
  
  # patch: apply the following patches. Each patch should be in git unified diff format (paths are relative to $WORKSPACE/$SCENARIODIR)
  patch:
    - somepatch.patch
  
  # exec: run AFTER other setup steps (ex: copy/patch). Each exec item is just a shell command to run in $WORKSPACE/$SCENARIODIR.
  exec:
    - git switch -c gab_tui_build && git add -A && git commit -m "update tests"

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
  
  # allow-multiple-turns-on-failed-verify: if the agent ends its turn before solving the problem, this allows it to continue.
  # When being prompted to continue, we will just send:
  # - The output of `verify`
  # - "Please continue until the problem is solved."
  # We ask the agent to continue IF verify does not pass AND this option is true.
  # We limit usage of this to 3 continues (may be configurable in future).
  allow-multiple-turns-on-failed-verify: true

  # FUTURE IDEAS:
  # plan: true # let planning agents actually do their /plan feature. Non-planning agents are told a generic "make a plan" instruction.
  #
  # PER-AGENT-CONFIG:
  # - we may want a way to specifically give certain agents slightly modified instructions. We could do that here.
  # - But, gotta be careful we keep things fair, we want minimal instruction drift. Need to be clear about use case.
    
# verify: how we know if the agent succeeded in this scenario.
# There are two levels of success: complete success and partial success. Partial success is a number from 0 to 1 (ex: 0.84). Complete success is a 1.
# For complete success, all checks and tests listed must pass (tests not mentioned need not pass).
# For partial success:
# - In order to score partial success, everything except `partial-tests` must pass, including other tests.
# - Partial success is only possible if explicitly enabled via `partial-tests:`, which lists specific tests (not all scenarios really lend themselves to partial success).
# - We analyze the tests run within `partial-tests`. Each test that Go runs which has a discrete PASS/FAIL/ERROR result reported counts as a test.
# - (In other words, usually it's TestXxx. But when the test uses t.Run, often in a table driven test, each t.Run also counts as a test).
verify:

  # Must modify these files or directories. Failure to modify these results in a verify -> failed.
  # If an element is a directory (optional slash allowed), we can only modify/create/delete files directly in that directory (nonrecursively). Supports globs.
  must-modify:
    - internal/q/tui

  # May not modify any of these files/dirs/globs.
  no-modify:
    - internal/q/tui/golden*
    - internal/q/tui/SPEC.md

  # copy: an array of from/to pairs. Can be used to copy test files the agent didn't see when running.
  # Any copied file is removed when verify is finished.
  # NOTE: copy does not play well with allow-multiple-turns-on-failed-verify: true, since we'd be sharing test failures
  # that the agent can't see.
  copy:
    - from: some_test.go # relative to scenario directory in `testdata`
      to: path/to/package # relative to $WORKSPACE/$SCENARIODIR. Can be ".".

  # tests is a list of must-pass tests (partial success not relevant). All elements are run with `go test`.
  # Each element is:
  # - a relative directory (relative to `$WORKSPACE/$SCENARIODIR`).
  # - a relative file (of a _test.go file).
  # - a glob of test files (ex: internal/q/tui/golden*_test.go)
  # - a Go-style package pattern (ex: ./...; ./foo; ./bar/...)
  # - If the element resolves such that there's only one target Go package, you may use -run to indicate specific tests are run. (-run must come last; only one; not --run)
  tests:
    - some/pkg
    - ./other/...
    - internal/app/golden_*_test.go
    - internal/app/some_test.go
    - ./mypkg -run TestImportant
    - ./mypkg -run=TestImportant
    - ./mypkg -run "TestImportant|TestThing"
    - ./mypkg -run 'TestImportant/^(Sub1|Sub2)$'

  # partial-tests: which set of tests do we consider for partial success. When partial success is not relevant, can omit this field.
  # This array uses the same format as `tests`.
  partial-tests:
    - internal/q/tui/golden*

  # FUTURE:
  # - we may want custom verification scripts
  # script: myscript.sh
```

## agents.yml and llms.yml

Supported agents and their LLMs must be listed in these yml files. For each agent and its LLMs, we'll need a harness that knows how to execute it with specific parameters and extract transcripts and token usage.

Some agents may be "manual" -- their harness will just indicate that a human should go run the agent. These manually run agents should still be listed in agents.yml and llms.yml.

## Docker and containers

Docker/containerization is mostly orthogonal. This softare will run on any computer.

Its recommended to run in a container. Run ./docker_dev.sh to be dropped in a shell in a container. From there, run the commands. Details:
- Relevant keys (ex: OPENAI_API_KEY will be copied to docker environment).
- If `GOAGENTBENCH_CODEX_MOUNT_AUTH` is set, we'll mount ~/.codex/auth.json to the container so it can use those credentials.
- Results from inside the container will be written to `./results` in your host computer (via mount).

The only non-orthogonal piece: agent versions in agents.yml must manually be kept up to date with installed agents in `Dockerfile`.
