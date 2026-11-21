#!/usr/bin/env python3
"""Summarise stochadex json_log runs and test claims against them.

A json_log holds one JSON object per line: {"partition_name", "state", "time"}.
Each log given is one run: one member of an ensemble.

  run_report.py summarise LOG [LOG ...] [--from-time T] [--to-time T] [--json]
  run_report.py check --claims claims.json LOG [LOG ...] [--json]

summarise prints, for every element of every partition, its first and final
value, minimum, maximum and time-average: each the mean across members, with
the final value's spread across members.

check tests each claim in a JSON list and exits 1 when any fails. A claim is

  {"claim": "the reservoir never empties",
   "partition": "reservoir", "index": 0,        index defaults to 0
   "stat": "min",                               first|final|min|max|mean|std|fraction
   "when": ["<=", 0],                           with fraction: the rows that count
   "op": ">", "value": 0,                       < <= > >= between approx
   "from_time": 20, "to_time": 100,             optional window, inclusive
   "members": "all"}                            all (default) | mean | a fraction

between takes "value": [low, high]; approx takes "value" and "tol". fraction is
the share of the window's rows whose value satisfies "when". members
says how an ensemble must satisfy the claim: "all" members, the across-member
"mean" of the statistic, or at least the given fraction of members (0.9).
A value that is not finite fails every claim on it. Standard library only.
"""

import argparse
import json
import math
import sys

STATS = ("first", "final", "min", "max", "mean", "std", "fraction")
COMPARE = {
    "<": lambda a, b: a < b,
    "<=": lambda a, b: a <= b,
    ">": lambda a, b: a > b,
    ">=": lambda a, b: a >= b,
}


def read_log(path):
    """Returns {partition: [(time, state), ...]} in file order."""
    series = {}
    with open(path) as handle:
        for number, line in enumerate(handle, 1):
            line = line.strip()
            if not line:
                continue
            try:
                row = json.loads(line)
                name, state, time = row["partition_name"], row["state"], row["time"]
            except (ValueError, KeyError, TypeError) as error:
                sys.exit(f"{path}:{number}: not a json_log row ({error})")
            series.setdefault(name, []).append((float(time), [as_float(v) for v in state]))
    if not series:
        sys.exit(f"{path}: no rows")
    return series


def as_float(value):
    # The engine writes NaN and infinities as strings or nulls in JSON.
    try:
        return float(value)
    except (TypeError, ValueError):
        return math.nan


def stats_of(rows, index, from_time=None, to_time=None, when=None):
    """One member's statistics of element index over the window, or None."""
    values = [
        state[index]
        for time, state in rows
        if (from_time is None or time >= from_time)
        and (to_time is None or time <= to_time)
        and index < len(state)
    ]
    if not values:
        return None
    mean = math.fsum(values) / len(values)
    variance = math.fsum((v - mean) ** 2 for v in values) / len(values)
    return {
        "first": values[0],
        "final": values[-1],
        "min": min(values),
        "max": max(values),
        "mean": mean,
        "std": math.sqrt(variance),
        "rows": len(values),
        "finite": all(math.isfinite(v) for v in values),
        "fraction": sum(COMPARE[when[0]](v, when[1]) for v in values) / len(values)
        if when
        else math.nan,
    }


def mean_sd(values):
    mean = math.fsum(values) / len(values)
    if len(values) < 2:
        return mean, 0.0
    return mean, math.sqrt(math.fsum((v - mean) ** 2 for v in values) / (len(values) - 1))


def fmt(value):
    return f"{value:.6g}"


def summarise(args):
    members = [read_log(path) for path in args.logs]
    names = list(members[0])
    for member in members[1:]:
        names += [name for name in member if name not in names]
    table, not_finite = [], []
    for name in names:
        width = max(len(rows[0][1]) for rows in (m.get(name) for m in members) if rows)
        for index in range(width):
            per_member = [
                s
                for s in (
                    stats_of(m[name], index, args.from_time, args.to_time)
                    for m in members
                    if name in m
                )
                if s
            ]
            if not per_member:
                continue
            if not all(s["finite"] for s in per_member):
                not_finite.append(f"{name}[{index}]")
            final, spread = mean_sd([s["final"] for s in per_member])
            table.append(
                {
                    "partition": name,
                    "index": index,
                    "members": len(per_member),
                    "rows": per_member[0]["rows"],
                    "first": mean_sd([s["first"] for s in per_member])[0],
                    "final": final,
                    "final_sd_across_members": spread,
                    "min": min(s["min"] for s in per_member),
                    "max": max(s["max"] for s in per_member),
                    "mean": mean_sd([s["mean"] for s in per_member])[0],
                }
            )
    if args.json:
        json.dump({"series": table, "not_finite": not_finite}, sys.stdout, indent=1)
        print()
    else:
        print(f"{len(members)} member(s)")
        print("| series | rows | first | final | final ± across members | min | max | time-mean |")
        print("|---|---|---|---|---|---|---|---|")
        for row in table:
            print(
                f"| {row['partition']}[{row['index']}] | {row['rows']} | {fmt(row['first'])} "
                f"| {fmt(row['final'])} | {fmt(row['final_sd_across_members'])} "
                f"| {fmt(row['min'])} | {fmt(row['max'])} | {fmt(row['mean'])} |"
            )
        if not_finite:
            print("NOT FINITE (NaN or infinite values): " + ", ".join(not_finite))
    return 1 if not_finite else 0


def holds(op, observed, claim):
    if not math.isfinite(observed):
        return False
    value = claim["value"]
    if op in COMPARE:
        return COMPARE[op](observed, value)
    if op == "between":
        return value[0] <= observed <= value[1]
    if op == "approx":
        return abs(observed - value) <= claim["tol"]
    sys.exit(f"claim {claim.get('claim')!r}: unknown op {op!r}")


def check(args):
    with open(args.claims) as handle:
        claims = json.load(handle)
    members = [read_log(path) for path in args.logs]
    results = []
    for claim in claims:
        for key in ("claim", "partition", "stat", "op", "value"):
            if key not in claim:
                sys.exit(f"claim {claim.get('claim', claim)!r} has no {key!r}")
        if claim["stat"] not in STATS:
            sys.exit(f"claim {claim['claim']!r}: stat must be one of {', '.join(STATS)}")
        when = claim.get("when")
        if (claim["stat"] == "fraction") != (when is not None):
            sys.exit(f"claim {claim['claim']!r}: a fraction, and only a fraction, takes a when")
        if when is not None and (len(when) != 2 or when[0] not in COMPARE):
            sys.exit(f"claim {claim['claim']!r}: when is [op, value], op one of < <= > >=")
        if claim["op"] == "approx" and "tol" not in claim:
            sys.exit(f"claim {claim['claim']!r}: approx needs a tol")
        name, index = claim["partition"], claim.get("index", 0)
        observed = []
        for path, member in zip(args.logs, members):
            if name not in member:
                sys.exit(f"claim {claim['claim']!r}: {path} has no partition {name!r}")
            stats = stats_of(
                member[name], index, claim.get("from_time"), claim.get("to_time"), when
            )
            if stats is None:
                sys.exit(f"claim {claim['claim']!r}: {name}[{index}] has no rows in its window")
            observed.append(stats[claim["stat"]] if stats["finite"] else math.nan)
        mode = claim.get("members", "all")
        passing = [holds(claim["op"], value, claim) for value in observed]
        if mode == "all":
            passed = all(passing)
        elif mode == "mean":
            passed = holds(claim["op"], mean_sd(observed)[0], claim)
        elif isinstance(mode, (int, float)) and 0 <= mode <= 1:
            passed = sum(passing) / len(passing) >= mode
        else:
            sys.exit(f"claim {claim['claim']!r}: members must be all, mean or a fraction")
        results.append(
            {
                "claim": claim["claim"],
                "passed": passed,
                "series": f"{name}[{index}]",
                "stat": claim["stat"],
                "op": claim["op"],
                "value": claim["value"],
                "members": mode,
                "observed_mean": mean_sd(observed)[0],
                "observed_min": min(observed),
                "observed_max": max(observed),
                "members_passing": sum(passing),
                "members_total": len(passing),
            }
        )
    if args.json:
        json.dump(results, sys.stdout, indent=1)
        print()
    else:
        for result in results:
            print(
                f"{'PASS' if result['passed'] else 'FAIL'}  {result['claim']}: "
                f"{result['stat']} of {result['series']} {result['op']} {result['value']} "
                f"({result['members']}); observed {fmt(result['observed_min'])} to "
                f"{fmt(result['observed_max'])}, mean {fmt(result['observed_mean'])}; "
                f"{result['members_passing']}/{result['members_total']} members"
            )
    return 0 if all(result["passed"] for result in results) else 1


def main():
    parser = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    commands = parser.add_subparsers(dest="command", required=True)
    one = commands.add_parser("summarise", help="statistics of every series")
    one.add_argument("logs", nargs="+", metavar="LOG")
    one.add_argument("--from-time", type=float)
    one.add_argument("--to-time", type=float)
    one.add_argument("--json", action="store_true")
    one.set_defaults(run=summarise)
    two = commands.add_parser("check", help="test claims; exit 1 when one fails")
    two.add_argument("--claims", required=True)
    two.add_argument("logs", nargs="+", metavar="LOG")
    two.add_argument("--json", action="store_true")
    two.set_defaults(run=check)
    args = parser.parse_args()
    sys.exit(args.run(args))


if __name__ == "__main__":
    main()
