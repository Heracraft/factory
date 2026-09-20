#!/bin/sh
# Nightly pg_dump into /backups, pruned after BACKUP_KEEP_DAYS. Runs in the
# postgres:16-alpine image (busybox sh, no cron), so the schedule is a loop
# that sleeps until the next BACKUP_HOUR_UTC. `pg-backup once` dumps now,
# which is how a manual backup and the restore rehearsal start.
set -eu

hour=${BACKUP_HOUR_UTC:-2}
keep=${BACKUP_KEEP_DAYS:-35}

dump() {
  stamp=$(date -u +%Y%m%dT%H%M%SZ)
  out="/backups/repose-$stamp.dump"
  tmp="$out.partial"
  # -Fc: custom format, what `pg_restore` in the runbook reads; compressed.
  pg_dump -Fc -Z 6 -f "$tmp" && mv "$tmp" "$out"
  echo "pg-backup: wrote $out ($(stat -c %s "$out") bytes)"
  find /backups -name 'repose-*.dump' -mtime +"$keep" -print -delete | sed 's/^/pg-backup: pruned /'
}

case "${1:-loop}" in
  once) dump ;;
  loop)
    while :; do
      now=$(date -u +%s)
      today=$(( now / 86400 * 86400 + hour * 3600 ))
      if [ "$now" -lt "$today" ]; then next=$today; else next=$(( today + 86400 )); fi
      echo "pg-backup: next dump at $(date -u -d "@$next" +%FT%TZ 2>/dev/null || echo "$next")"
      sleep $(( next - now ))
      dump || echo "pg-backup: dump failed" >&2
    done
    ;;
  *) echo "usage: pg-backup [once|loop]" >&2; exit 2 ;;
esac
