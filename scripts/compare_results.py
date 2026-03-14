#!/usr/bin/env python3
"""Compare benchmark result summaries across arbitrary (agent, model) pairs."""

from __future__ import annotations

import argparse
import json
from dataclasses import dataclass
from pathlib import Path
from statistics import mean
from typing import Iterable


@dataclass(frozen=True)
class Pair:
    agent: str
    model: str

    @property
    def label(self) -> str:
        return f"{self.agent}@{self.model}"


@dataclass(frozen=True)
class Run:
    scenario: str
    pair: Pair
    run_id: str
    file_name: str
    started_at: str
    verified_at: str
    success: bool
    cost: float
    duration_seconds: float


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description=(
            "Summarize benchmark costs and durations per scenario for one or more "
            "(agent, model) pairs."
        )
    )
    parser.add_argument(
        "--results-dir",
        type=Path,
        default=Path("results"),
        help="Directory containing *.verify.json result files. Default: %(default)s",
    )
    parser.add_argument(
        "--pair",
        nargs=2,
        action="append",
        metavar=("AGENT", "MODEL"),
        required=True,
        help=(
            "Pair to compare. Repeat this flag to compare multiple tuples, "
            "for example: --pair codex gpt-5.4-high --pair codalotl gpt-5.4-high"
        ),
    )
    parser.add_argument(
        "--scenario",
        action="append",
        default=[],
        help=(
            "Optional scenario filter. Repeat to include multiple exact scenario names. "
            "Default: include all scenarios."
        ),
    )
    parser.add_argument(
        "--limit",
        type=int,
        default=None,
        help=(
            "Keep only the most recent N runs per scenario/pair. "
            "Default: use all matching runs."
        ),
    )
    parser.add_argument(
        "--union",
        action="store_true",
        help=(
            "Show the union of scenarios found in any supplied pair. "
            "Default: only show scenarios that have data for every supplied pair."
        ),
    )
    parser.add_argument(
        "--show-files",
        action="store_true",
        help="Print the source result filenames used for each summary line.",
    )
    return parser.parse_args()


def load_runs(results_dir: Path) -> list[Run]:
    runs: list[Run] = []
    for path in sorted(results_dir.rglob("*.verify.json")):
        try:
            payload = json.loads(path.read_text())
        except (OSError, json.JSONDecodeError):
            continue

        progress = payload.get("progress") or {}
        token_usage = progress.get("token_usage") or {}
        scenario = payload.get("scenario")
        agent = payload.get("agent")
        model = payload.get("model")
        cost = token_usage.get("cost")
        duration = progress.get("duration_seconds")
        if not isinstance(scenario, str) or not isinstance(agent, str) or not isinstance(model, str):
            continue
        if not isinstance(cost, (int, float)) or not isinstance(duration, (int, float)):
            continue

        runs.append(
            Run(
                scenario=scenario,
                pair=Pair(agent=agent, model=model),
                run_id=str(payload.get("run_id") or ""),
                file_name=path.name,
                started_at=str(progress.get("started_at") or payload.get("started_at") or ""),
                verified_at=str(payload.get("verified_at") or ""),
                success=bool(payload.get("success")),
                cost=float(cost),
                duration_seconds=float(duration),
            )
        )
    return runs


def apply_filters(
    runs: Iterable[Run],
    pairs: list[Pair],
    scenarios: set[str],
    limit: int | None,
) -> dict[str, dict[Pair, list[Run]]]:
    pair_set = set(pairs)
    grouped: dict[str, dict[Pair, list[Run]]] = {}

    for run in runs:
        if run.pair not in pair_set:
            continue
        if scenarios and run.scenario not in scenarios:
            continue
        grouped.setdefault(run.scenario, {}).setdefault(run.pair, []).append(run)

    for scenario_map in grouped.values():
        for pair, pair_runs in list(scenario_map.items()):
            pair_runs.sort(key=lambda run: (run.started_at, run.verified_at, run.file_name))
            if limit is not None:
                scenario_map[pair] = pair_runs[-limit:]
    return grouped


def selected_scenarios(
    grouped: dict[str, dict[Pair, list[Run]]], pairs: list[Pair], use_union: bool
) -> list[str]:
    if use_union:
        return sorted(grouped)
    return sorted(
        scenario
        for scenario, scenario_map in grouped.items()
        if all(pair in scenario_map and scenario_map[pair] for pair in pairs)
    )


def format_summary(runs: list[Run]) -> str:
    prices = ", ".join(f"{run.cost:.2f}" for run in runs)
    times = ", ".join(f"{run.duration_seconds:.0f}" for run in runs)
    avg_price = mean(run.cost for run in runs)
    avg_time = mean(run.duration_seconds for run in runs)
    success_count = sum(1 for run in runs if run.success)
    return (
        f"prices: {prices} (avg: {avg_price:.2f})    "
        f"times: {times} (avg: {avg_time:.0f})    "
        f"success: {success_count}/{len(runs)}"
    )


def main() -> int:
    args = parse_args()
    pairs = [Pair(agent=agent, model=model) for agent, model in args.pair]
    if len(set(pairs)) != len(pairs):
        raise SystemExit("duplicate --pair values are not allowed")
    if args.limit is not None and args.limit <= 0:
        raise SystemExit("--limit must be greater than zero")

    runs = load_runs(args.results_dir)
    grouped = apply_filters(runs, pairs, set(args.scenario), args.limit)
    scenarios = selected_scenarios(grouped, pairs, args.union)

    if not scenarios:
        raise SystemExit("no matching scenarios found")

    for scenario in scenarios:
        print(f"{scenario}:")
        scenario_map = grouped[scenario]
        for pair in pairs:
            runs_for_pair = scenario_map.get(pair, [])
            if not runs_for_pair:
                print(f"    {pair.label}: no runs")
                continue
            print(f"    {pair.label}: {format_summary(runs_for_pair)}")
            if args.show_files:
                file_list = ", ".join(run.file_name for run in runs_for_pair)
                print(f"        files: {file_list}")
        print()

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
