#!/usr/bin/env python3
"""Check the generated dashboards, and emit their PromQL as a rules file.

Grafana accepts almost any JSON and reports a broken panel by rendering it
empty with a small red triangle, which nobody clicks. These are the mistakes
that are worth failing a build for:

  * a panel with no query, or a query with no expression
  * a datasource uid that is not one of the three provisioned ones
  * a Postgres panel whose SQL names a table or column that
    docs/interfaces/db-schema.md does not have
  * a per-project Prometheus query, which is the label rule of
    docs/workstreams/10-observability.md §5
  * two dashboards with the same uid, or a panel with no title

With --emit-rules it prints every Prometheus expression as a Prometheus
recording-rule file, with Grafana's variables substituted, so that
`promtool check rules` judges whether each one parses. That is the closest
thing to a PromQL linter that does not need a running Prometheus.
"""

from __future__ import annotations

import argparse
import json
import pathlib
import re
import sys

HERE = pathlib.Path(__file__).resolve().parent
DOCS = HERE.parent.parent / "docs"

ALLOWED_UIDS = {"repose-prometheus", "repose-loki", "repose-postgres"}

# Labels a Prometheus query may not select on: §5 keeps per-project figures
# out of Prometheus entirely.
FORBIDDEN_LABELS = ("project_id", "guest_id", "user_id", "slug")

# Grafana's own variables, with a value that parses as PromQL.
VAR_SUBSTITUTIONS = {
    "$host_id": "host-01",
    "$project_id": "00000000-0000-0000-0000-000000000000",
    "$__range": "1h",
    "$__interval": "1m",
    "$__rate_interval": "1m",
}


def tables_and_columns() -> dict[str, set[str]]:
    """Read the table and column names out of docs/interfaces/db-schema.md.

    The schema block there is the contract; a dashboard that queries a column
    it does not have is a dashboard that breaks on the day it is needed.
    """
    text = (DOCS / "interfaces" / "db-schema.md").read_text()
    block = text.split("```sql", 1)[1].split("```", 1)[0]
    out: dict[str, set[str]] = {}
    # Each table is `name (col type, col type, ...)` possibly over several
    # lines, with comments after `--`.
    block = re.sub(r"--[^\n]*", "", block)
    for m in re.finditer(r"^(\w+)\s*\(([^;]*?)\)\s*$", block, re.M | re.S):
        name, body = m.group(1), m.group(2)
        cols = set()
        for part in re.split(r",", body):
            part = part.strip()
            if not part or part.startswith(("primary key", "unique", "partition")):
                continue
            col = part.split()[0]
            if col.isidentifier():
                cols.add(col)
        out[name] = cols
    return out


def load() -> list[tuple[pathlib.Path, dict]]:
    return [(p, json.loads(p.read_text())) for p in sorted(HERE.glob("*.json"))]


def panels_of(dash: dict):
    for p in dash.get("panels", []):
        if p.get("type") == "row":
            continue
        yield p


def check() -> list[str]:
    problems: list[str] = []
    schema = tables_and_columns()
    uids: dict[str, str] = {}
    for path, dash in load():
        name = path.name
        uid = dash.get("uid")
        if not uid:
            problems.append(f"{name}: no uid")
        elif uid in uids:
            problems.append(f"{name}: uid {uid} is also {uids[uid]}")
        else:
            uids[uid] = name
        if not dash.get("title", "").startswith("repose / "):
            problems.append(f"{name}: title {dash.get('title')!r} does not start with 'repose / '")
        if "repose" not in dash.get("tags", []):
            problems.append(f"{name}: not tagged repose")
        if not dash.get("description"):
            problems.append(f"{name}: no description")
        seen_ids = set()
        for p in panels_of(dash):
            title = p.get("title") or "(untitled)"
            where = f"{name}: panel {title!r}"
            if not p.get("title"):
                problems.append(f"{where}: no title")
            if p.get("id") in seen_ids:
                problems.append(f"{where}: duplicate panel id {p.get('id')}")
            seen_ids.add(p.get("id"))
            if not p.get("description"):
                problems.append(f"{where}: no description; a panel nobody can explain is a panel nobody trusts")
            targets = p.get("targets") or []
            if not targets:
                problems.append(f"{where}: no query")
            for t in targets:
                ds = (t.get("datasource") or {}).get("uid")
                if ds not in ALLOWED_UIDS:
                    problems.append(f"{where}: datasource uid {ds!r} is not provisioned")
                if ds == "repose-postgres":
                    problems += check_sql(where, t.get("rawSql", ""), schema)
                elif ds == "repose-prometheus":
                    problems += check_promql(where, t.get("expr", ""))
                elif ds == "repose-loki":
                    if not t.get("expr"):
                        problems.append(f"{where}: empty Loki query")
    return problems


def check_promql(where: str, expr: str) -> list[str]:
    out = []
    if not expr.strip():
        return [f"{where}: empty Prometheus query"]
    for bad in FORBIDDEN_LABELS:
        if re.search(rf"\b{bad}\s*=", expr):
            out.append(f"{where}: selects on {bad}, which is never a Prometheus label (§5)")
    for m in re.finditer(r"\b(repose_[a-z0-9_]+)", expr):
        metric = m.group(1)
        base = re.sub(r"_(bucket|count|sum)$", "", metric)
        if not base.startswith("repose_"):
            out.append(f"{where}: {metric} is not in the repose_ namespace")
    return out


def check_sql(where: str, sql: str, schema: dict[str, set[str]]) -> list[str]:
    out = []
    if not sql.strip():
        return [f"{where}: empty SQL query"]
    lowered = sql.lower()
    used_tables = set(re.findall(r"\b(?:from|join)\s+([a-z_]+)", lowered)) - {"lateral"}
    for t in used_tables:
        if t not in schema:
            out.append(f"{where}: table {t} is not in docs/interfaces/db-schema.md")
    # Only check columns of the tables this file knows, and only bare
    # qualified references (alias.column), which is what a typo shows up as.
    aliases = dict(
        (alias, table)
        for table, alias in re.findall(r"\b(?:from|join)\s+([a-z_]+)\s+([a-z])\b", lowered)
        if table != "lateral"
    )
    for alias_col in re.findall(r"\b([a-z])\.([a-z_]+)\b", lowered):
        alias, col = alias_col
        table = aliases.get(alias)
        if not table or table not in schema:
            continue
        if col not in schema[table] and col not in ("id",):
            out.append(f"{where}: {table}.{col} is not in db-schema.md")
    return out


def emit_rules() -> str:
    """Every Prometheus expression as a recording rule, for promtool."""
    lines = ["# Generated by ops/dashboards/validate.py --emit-rules. Not deployed:",
             "# this file exists so promtool parses every panel's PromQL.",
             "groups:"]
    for path, dash in load():
        group = path.stem.replace(".", "-")
        rules = []
        for i, p in enumerate(panels_of(dash)):
            for j, t in enumerate(p.get("targets") or []):
                if (t.get("datasource") or {}).get("uid") != "repose-prometheus":
                    continue
                expr = t.get("expr", "")
                for var, val in VAR_SUBSTITUTIONS.items():
                    expr = expr.replace(var, val)
                # A rules file cannot hold a newline-bearing expression
                # inline, so it is folded; PromQL ignores the whitespace.
                expr = " ".join(expr.split())
                rules.append((f"dashboard:{group}:p{i}_{j}", expr))
        if not rules:
            continue
        lines.append(f"  - name: {group}")
        lines.append("    rules:")
        for record, expr in rules:
            lines.append(f"      - record: {record}")
            lines.append(f"        expr: {json.dumps(expr)}")
    return "\n".join(lines) + "\n"


def query_panels(base: str, host_id: str = "host-01") -> int:
    """Run every Prometheus panel query against a live Prometheus.

    Development only, and the last check before a dashboard is believed: a
    query can parse, name real metrics and still return nothing, because a
    label does not match or an aggregation drops it. Against the dev stack
    (ops/dev/docker-compose.yml plus `go run ./ops/dev/seedmetrics`) every
    panel should have data; against the real Prometheus, a panel with no data
    is either a quiet fleet or a broken query.
    """
    import urllib.parse
    import urllib.request

    empty = 0
    total = 0
    for path, dash in load():
        for p in panels_of(dash):
            for t in p.get("targets") or []:
                if (t.get("datasource") or {}).get("uid") != "repose-prometheus":
                    continue
                expr = t.get("expr", "")
                subs = dict(VAR_SUBSTITUTIONS, **{"$host_id": host_id})
                for var, val in subs.items():
                    expr = expr.replace(var, val)
                total += 1
                url = base.rstrip("/") + "/api/v1/query?" + urllib.parse.urlencode({"query": expr})
                with urllib.request.urlopen(url, timeout=10) as r:  # noqa: S310 (a localhost dev stack)
                    body = json.load(r)
                n = len(body.get("data", {}).get("result", []))
                mark = "ok  " if n else "EMPTY"
                if not n:
                    empty += 1
                print(f'{mark} {path.name}: {p["title"]!r}: {n} series')
    print(f"\n{total} Prometheus panel queries, {empty} with no data")
    return 1 if empty else 0


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--emit-rules", action="store_true", help="print the panels' PromQL as a rules file")
    ap.add_argument("--query", metavar="URL", help="run every Prometheus panel query against this Prometheus and report empty results")
    ap.add_argument("--host-id", default="host-01", help="value for the $host_id variable when querying (the dev stack labels its target dev-host)")
    args = ap.parse_args()
    if args.emit_rules:
        sys.stdout.write(emit_rules())
        return 0
    if args.query:
        return query_panels(args.query, args.host_id)
    problems = check()
    for p in problems:
        print(p, file=sys.stderr)
    if problems:
        print(f"{len(problems)} problem(s)", file=sys.stderr)
        return 1
    dashboards = load()
    panels = sum(len(list(panels_of(d))) for _, d in dashboards)
    print(f"{len(dashboards)} dashboards, {panels} panels, every query readable")
    return 0


if __name__ == "__main__":
    sys.exit(main())
