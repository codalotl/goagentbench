# smoke

This directory contains scenarios that are intended to be used for the **development of goagentbench**. They are not intended to make it into any benchmark.

They can be considered smoke/integration tests.

For instance, they might contain:
- a scenario that always fails with allow-multiple-turns-on-failed-verify: true. -> tests that we give the agent 3 retries.
- hidden partial tests, to validate that machinery.
- etc