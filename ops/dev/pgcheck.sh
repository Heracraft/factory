#!/usr/bin/env bash
# Run every Postgres query in ops/dashboards/ against a real Postgres, and
# exercise the partition maintenance of ops/sql/partitions.sql.
#
#   docker compose -f ops/dev/docker-compose.yml up -d postgres
#   ops/dev/pgcheck.sh
#
# Two things this catches that nothing else does:
#
#   * a panel's SQL that does not run. The validator in
#     ops/dashboards/validate.py reads db-schema.md and catches a wrong table
#     or column; only Postgres catches a `group by` that does not cover the
#     select list, a jsonb operator on the wrong type, or an ambiguous column.
#   * a retention that never deletes anything. docs/CHECKLIST.md asks for a
#     log of one partition drop; this makes one happen.
#
# It uses the fixture schema (ops/dev/schema-fixture.sql), because workstream
# 05 owns the migrations and they do not exist yet.
set -euo pipefail

cd "$(dirname "$0")/../.."

PGHOST=${PGHOST:-127.0.0.1}
PGPORT=${PGPORT:-55432}
PGUSER=${PGUSER:-postgres}
PGPASSWORD=${PGPASSWORD:-repose}
PGDATABASE=${PGDATABASE:-repose}
export PGHOST PGPORT PGUSER PGPASSWORD PGDATABASE

psql() {
  if command -v psql >/dev/null 2>&1; then
    command psql -v ON_ERROR_STOP=1 "$@"
  else
    nix shell nixpkgs#postgresql_16 -c psql -v ON_ERROR_STOP=1 "$@"
  fi
}

say() { printf '\n== %s ==\n' "$1"; }

say "waiting for Postgres on $PGHOST:$PGPORT"
for _ in $(seq 1 60); do
  if psql -qtAc 'select 1' >/dev/null 2>&1; then break; fi
  sleep 1
done
psql -qtAc 'select version()'

say "fixture schema and the partition functions"
psql -q -f ops/dev/schema-fixture.sql
psql -q -f ops/sql/partitions.sql

say "partitions: backfill, then maintain"
psql -c 'select * from repose_partitions_backfill()'
psql -c 'select * from repose_partitions_maintain()'

say "synthetic data"
psql -q <<'SQL'
truncate usage_hours, snapshots, proc_samples, meter_samples cascade;
delete from projects; delete from users; delete from hosts;

insert into users (id, handle, email, billing_status, tz)
values ('00000000-0000-7000-8000-000000000001', 'heracraft', 'owner@example.invalid', 'exempt', 'UTC');

insert into hosts (id, hostname, sku, provider, region, mem_bytes, vcpus, pool_bytes, state, last_heartbeat_at)
values ('00000000-0000-7000-8000-0000000000a1', 'host-01', 'Standard_D16s_v7', 'azure', 'eastus',
        68719476736, 16, 549755813888, 'ready', now());

insert into projects (id, user_id, name, slug, remote_url, class, state, host_id, volume_bytes, started_at)
values
 ('00000000-0000-7000-8000-000000000101', '00000000-0000-7000-8000-000000000001', 'todo-app', 'todo-app',
  'github.com/heracraft/todo-app', 'large', 'running', '00000000-0000-7000-8000-0000000000a1', 42949672960, now() - interval '3 days'),
 ('00000000-0000-7000-8000-000000000102', '00000000-0000-7000-8000-000000000001', 'miner-victim', 'miner-victim',
  'github.com/heracraft/other', 'small', 'running', '00000000-0000-7000-8000-0000000000a1', 21474836480, now() - interval '5 days');

-- 24 hours of minute samples for both projects: one working normally, one at
-- full CPU with nobody attached and a terabyte of egress (what the Abuse
-- dashboard is looking for).
insert into meter_samples (ts, project_id, host_id, state, class, cpu_ns, mem_rss, net_tx, net_rx,
                           disk_alloc, disk_used, ssh_sessions, tmux_clients, agents, docker_containers, guestd_ok)
select
  ts,
  '00000000-0000-7000-8000-000000000101',
  '00000000-0000-7000-8000-0000000000a1',
  'running', 'large',
  (random() * 30e9)::bigint, 2147483648, (random() * 2e6)::bigint, (random() * 5e6)::bigint,
  42949672960, 18253611008, 1, 1,
  '[{"agent":"claude","window":"claude","state":"working"}]'::jsonb, 2,
  ts < now() - interval '4 hours' or ts > now() - interval '3 hours'
from generate_series(now() - interval '24 hours', now(), interval '1 minute') ts;

insert into meter_samples (ts, project_id, host_id, state, class, cpu_ns, mem_rss, net_tx, net_rx,
                           disk_alloc, disk_used, ssh_sessions, tmux_clients, agents, docker_containers, guestd_ok)
select
  ts,
  '00000000-0000-7000-8000-000000000102',
  '00000000-0000-7000-8000-0000000000a1',
  'running', 'small',
  119e9::bigint, 4026531840, 800e6::bigint, 1e6::bigint,
  21474836480, 5368709120, 0, 0, '[]'::jsonb, 1, true
from generate_series(now() - interval '24 hours', now(), interval '1 minute') ts;

insert into proc_samples (ts, project_id, comm, cpu_ns, rss)
select ts, '00000000-0000-7000-8000-000000000101', comm, (random() * 20e9)::bigint, (random() * 1e9)::bigint
from generate_series(now() - interval '24 hours', now(), interval '5 minutes') ts,
     unnest(array['node', 'claude', 'docker', 'tmux', 'bash']) comm;

insert into proc_samples (ts, project_id, comm, cpu_ns, rss)
select ts, '00000000-0000-7000-8000-000000000102', 'xmrig', 59e9::bigint, 536870912
from generate_series(now() - interval '24 hours', now(), interval '5 minutes') ts;

insert into snapshots (id, project_id, host_id, blob_path, bytes, reason, taken_at)
values (gen_random_uuid(), '00000000-0000-7000-8000-000000000101', '00000000-0000-7000-8000-0000000000a1',
        'u/p/2026-09-19T03-00-00Z.img.zst', 3221225472, 'scheduled', now() - interval '20 hours'),
       (gen_random_uuid(), '00000000-0000-7000-8000-000000000101', '00000000-0000-7000-8000-0000000000a1',
        'u/p/2026-09-18T03-00-00Z.img.zst', 3113851289, 'scheduled', now() - interval '44 hours');

insert into usage_hours (project_id, hour, class, running_seconds, gb_alloc, egress_bytes, cost_cents)
select p.id, h, p.class, 3600, p.volume_bytes / (1024^3),
       (random() * 5e9)::bigint, case p.class when 'large' then 14 else 7 end
from projects p,
     generate_series(date_trunc('hour', now() - interval '47 hours'), date_trunc('hour', now()), interval '1 hour') h;
SQL
psql -c 'select count(*) as meter_samples, (select count(*) from proc_samples) as proc_samples from meter_samples'

say "every dashboard query runs"
python3 - <<'PY'
import glob, json, re, subprocess, sys, os

# Grafana's macros, as Postgres a server will accept. $__timeFilter becomes a
# real range so the query plans the way it will in Grafana.
MACROS = [
    (re.compile(r"\$__timeFilter\(([a-z_.]+)\)"), r"\1 between now() - interval '24 hours' and now()"),
    (re.compile(r"\$__timeGroup\(([a-z_.]+),\s*'?([a-z0-9]+)'?\)"), r"date_trunc('hour', \1)"),
    (re.compile(r"'\$project_id'"), "'00000000-0000-7000-8000-000000000101'"),
    (re.compile(r"\$project_id"), "00000000-0000-7000-8000-000000000101"),
    (re.compile(r"\$host_id"), "host-01"),
]

fails = 0
ran = 0
for path in sorted(glob.glob("ops/dashboards/*.json")):
    dash = json.load(open(path))
    for panel in dash["panels"]:
        for t in panel.get("targets", []):
            if (t.get("datasource") or {}).get("uid") != "repose-postgres":
                continue
            sql = t["rawSql"]
            for pat, rep in MACROS:
                sql = pat.sub(rep, sql)
            ran += 1
            p = subprocess.run(
                ["psql", "-v", "ON_ERROR_STOP=1", "-qtAc", sql],
                capture_output=True, text=True, env=dict(os.environ),
            )
            name = f'{os.path.basename(path)}: {panel["title"]!r}'
            if p.returncode != 0:
                fails += 1
                print(f"FAIL {name}\n  {sql}\n  {p.stderr.strip()}", file=sys.stderr)
            else:
                rows = [l for l in p.stdout.strip().split("\n") if l]
                print(f"ok   {name}: {len(rows)} row(s)")
print(f"\n{ran} Postgres panel queries, {fails} failed")
sys.exit(1 if fails else 0)
PY

say "retention: a partition older than the window is dropped"
psql -q <<'SQL'
-- A month that is entirely past proc_samples' 30-day retention, with a row in
-- it, so the drop has something to remove.
select repose_partition_create('proc_samples', (date_trunc('month', now()) - interval '3 months')::date);
insert into proc_samples (ts, project_id, comm, cpu_ns, rss)
values (date_trunc('month', now()) - interval '3 months' + interval '2 days',
        '00000000-0000-7000-8000-000000000101', 'old', 1, 1);
SQL
echo "-- partitions before:"
psql -qtAc "select relname from pg_class where relname like 'proc\\_samples\\_%' and relkind = 'r' order by 1"
echo "-- maintain:"
psql -c 'select * from repose_partitions_maintain()'
echo "-- partitions after:"
psql -qtAc "select relname from pg_class where relname like 'proc\\_samples\\_%' and relkind = 'r' order by 1"

printf '\nok\n'
