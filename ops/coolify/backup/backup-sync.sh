#!/bin/sh
# Copy finished dumps from /backups to R2 every 15 minutes. `copy`, never
# `sync`: nothing is deleted in the bucket from here; its lifecycle rule
# (infra/r2, 35 days) is the retention, so a wiped volume cannot wipe the
# bucket. Partial files are excluded by name.
set -u
bucket=${R2_BUCKET:-repose-pg-backups}
while :; do
  rclone copy /backups "r2:$bucket" --exclude '*.partial' --min-age 1m --no-traverse -q \
    && echo "backup-sync: $(date -u +%FT%TZ) synced" \
    || echo "backup-sync: $(date -u +%FT%TZ) rclone failed" >&2
  sleep 900
done
