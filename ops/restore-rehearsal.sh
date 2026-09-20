#!/usr/bin/env bash
# Restore a Postgres dump into a throwaway database and check it, timing
# each step.
#
#   ops/restore-rehearsal.sh backup.dmp          # a dump you downloaded
#   ops/restore-rehearsal.sh --from-bucket       # newest object in a bucket
#
# docs/CHECKLIST.md's release list wants "Postgres restore from R2
# rehearsed ... with the time recorded", and docs/ops/RUNBOOK.md
# "Postgres restore" is the procedure. A procedure somebody reconstructs
# under pressure is a procedure with a step missing, so this is that
# procedure as one command with a clock on it: the number it prints is
# what an incident will cost, and it can be re-run whenever the schema or
# the data volume changes rather than once before launch.
#
# **Where the dump comes from.** Coolify owns the backups and uploads them
# to an S3 storage configured in the owner's own Coolify; no credential
# for that storage is in this repository or on the control VM
# (DECISIONS I-102). So the normal path is: download the dump from that
# database's **Backups** tab in Coolify, and pass the file. `--from-bucket`
# stays for whoever does have an rclone remote — it is three lines — but it
# is not how production is backed up and nothing here will create one.
#
# It never touches the platform database. The restore target is a
# throwaway `postgres:16-alpine` container of its own, removed at the end
# (and on Ctrl-C), with no published port; nothing here connects to
# `repose-postgres` at all. That is stronger than the runbook's "never
# onto production" and it is why this is safe to run on the control VM.
#
# What it needs: docker, and the dump. `repose-admin db verify` runs from
# the api image by default (`--admin-image`), or from a binary named in
# REPOSE_ADMIN.
set -euo pipefail

BUCKET=${BUCKET:-repose-pg-backups}
REMOTE=${REMOTE:-r2}
CONTAINER=repose-restore-rehearsal
PGPASS=$(head -c 18 /dev/urandom | base64 | tr -d '/+=')
DUMP=""
ADMIN_IMAGE=""
FROM_BUCKET=0
KEEP=0
WORK=$(mktemp -d)

usage() {
	cat >&2 <<'EOF'
usage: ops/restore-rehearsal.sh <dump-file> [--admin-image IMAGE] [--keep]
       ops/restore-rehearsal.sh --from-bucket [--bucket NAME] [--remote NAME] ...

  <dump-file>    the dump to restore, downloaded from the database's
                 Backups tab in Coolify (--dump FILE also accepted)
  --from-bucket  fetch the newest object with rclone instead; needs a
                 remote you made yourself (production does not back up
                 this way, DECISIONS I-102)
  --bucket       bucket name for --from-bucket (default repose-pg-backups)
  --remote       rclone remote for --from-bucket (default r2)
  --admin-image  image carrying repose-admin (default: the running `api`
                 application's image, found through docker)
  --keep         leave the throwaway database running for poking at
EOF
	exit 2
}

while [ $# -gt 0 ]; do
	case $1 in
	--dump) DUMP=$2; shift 2 ;;
	--from-bucket) FROM_BUCKET=1; shift ;;
	--bucket) BUCKET=$2; shift 2 ;;
	--remote) REMOTE=$2; shift 2 ;;
	--admin-image) ADMIN_IMAGE=$2; shift 2 ;;
	--keep) KEEP=1; shift ;;
	-*) usage ;;
	# A bare path is the dump, which is the common case and should not
	# need a flag.
	*) [ -z "$DUMP" ] || usage; DUMP=$1; shift ;;
	esac
done
[ -n "$DUMP" ] || [ "$FROM_BUCKET" = 1 ] || usage

cleanup() {
	[ "$KEEP" = 1 ] || docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
	rm -rf "$WORK"
}
trap cleanup EXIT INT TERM

now() { date +%s; }
say() { printf '\n== %s ==\n' "$1"; }

# --- the dump ---------------------------------------------------------
t_fetch_start=$(now)
if [ "$FROM_BUCKET" = 1 ]; then
	say "newest dump in ${REMOTE}:${BUCKET}"
	command -v rclone >/dev/null || { echo "rclone is not installed; download the dump from Coolify's Backups tab and pass the file" >&2; exit 2; }
	newest=$(rclone lsjson --recursive "${REMOTE}:${BUCKET}" |
		jq -r 'sort_by(.ModTime) | last | .Path // empty')
	[ -n "$newest" ] || { echo "bucket ${BUCKET} is empty; nothing to rehearse" >&2; exit 1; }
	echo "$newest"
	rclone copyto "${REMOTE}:${BUCKET}/${newest}" "$WORK/dump" --progress
	DUMP="$WORK/dump"
fi
[ -f "$DUMP" ] || { echo "no such dump: $DUMP (download it from the database's Backups tab in Coolify)" >&2; exit 2; }
t_fetch=$(( $(now) - t_fetch_start ))
bytes=$(stat -c %s "$DUMP")

# Coolify's dumps are gzipped custom-format or plain SQL depending on how
# the backup was configured; decide by looking rather than by the name.
case "$(file -b --mime-type "$DUMP")" in
application/gzip | application/x-gzip)
	gzip -dc "$DUMP" >"$WORK/plain" && DUMP="$WORK/plain"
	;;
esac
kind=$(head -c 5 "$DUMP" | grep -q 'PGDMP' && echo custom || echo plain)
echo "dump: $(numfmt --to=iec "$bytes" 2>/dev/null || echo "$bytes B"), format $kind"

# --- the throwaway database -------------------------------------------
say "throwaway postgres"
docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
docker run -d --name "$CONTAINER" \
	-e POSTGRES_PASSWORD="$PGPASS" -e POSTGRES_DB=repose \
	postgres:16-alpine >/dev/null
for _ in $(seq 1 60); do
	docker exec "$CONTAINER" pg_isready -U postgres -q && break
	sleep 1
done
docker exec "$CONTAINER" pg_isready -U postgres -q || { echo "throwaway postgres never became ready" >&2; exit 1; }
echo "$CONTAINER up (no published port, removed on exit)"

# --- the restore ------------------------------------------------------
say "restore"
t_restore_start=$(now)
if [ "$kind" = custom ]; then
	docker exec -i "$CONTAINER" pg_restore -U postgres -d repose --no-owner --no-privileges <"$DUMP"
else
	docker exec -i "$CONTAINER" psql -U postgres -d repose -v ON_ERROR_STOP=1 -q <"$DUMP"
fi
t_restore=$(( $(now) - t_restore_start ))

# --- the check --------------------------------------------------------
say "repose-admin db verify"
ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}} {{end}}' "$CONTAINER" | awk '{print $1}')
dsn="postgres://postgres@${ip}:5432/repose?sslmode=disable"
t_verify_start=$(now)
if [ -n "${REPOSE_ADMIN:-}" ]; then
	DATABASE_URL="$dsn" PGPASSWORD="$PGPASS" "$REPOSE_ADMIN" db status
	DATABASE_URL="$dsn" PGPASSWORD="$PGPASS" "$REPOSE_ADMIN" db verify
else
	if [ -z "$ADMIN_IMAGE" ]; then
		# The api application's image, whatever Coolify last deployed.
		ADMIN_IMAGE=$(docker ps --format '{{.Image}}' --filter 'label=coolify.managed=true' |
			grep -v '^postgres' | head -1)
	fi
	[ -n "$ADMIN_IMAGE" ] || { echo "could not find an image with repose-admin; pass --admin-image or REPOSE_ADMIN" >&2; exit 2; }
	echo "using $ADMIN_IMAGE"
	for sub in status verify; do
		docker run --rm --network container:"$CONTAINER" \
			-e DATABASE_URL="postgres://postgres@127.0.0.1:5432/repose?sslmode=disable" \
			-e PGPASSWORD="$PGPASS" \
			--entrypoint /usr/local/bin/repose-admin "$ADMIN_IMAGE" db "$sub"
	done
fi
t_verify=$(( $(now) - t_verify_start ))

say "timing"
printf '  fetch   %4ds\n  restore %4ds\n  verify  %4ds\n  total   %4ds\n' \
	"$t_fetch" "$t_restore" "$t_verify" "$(( t_fetch + t_restore + t_verify ))"
echo
echo "Record the total in docs/CHECKLIST.md's Postgres-restore item, with the"
echo "date and the dump's size ($(numfmt --to=iec "$bytes" 2>/dev/null || echo "$bytes B"))."
