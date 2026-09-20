// Package scheduler is the placement function (DESIGN.md §9,
// 05-control-plane-api.md §5.3): among hosts that are ready, not draining
// and heartbeating, with room for the class's memory below the host
// reserve and pool space for the volume, pick the one with the most free
// memory. Placement runs under one advisory transaction lock so
// concurrent creates see each other's reservations.
package scheduler

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ErrNoCapacity is the `capacity` error: no host fits.
var ErrNoCapacity = errors.New("no host with capacity")

// HeartbeatWindow is how long a host may be silent and still take work.
const HeartbeatWindow = 90 * time.Second

// LockPlacement is the advisory lock serialising placements.
const LockPlacement int64 = 1100

// ClassRAM is the memory a class reserves (DESIGN.md §5).
func ClassRAM(class string) int64 {
	switch class {
	case "small":
		return 4 << 30
	case "xl":
		return 16 << 30
	default:
		return 8 << 30
	}
}

// ClassVCPU is the vCPU count of a class.
func ClassVCPU(class string) int {
	switch class {
	case "small":
		return 2
	case "xl":
		return 8
	default:
		return 4
	}
}

// DefaultVolume is the default thin volume per class.
func DefaultVolume(class string) int64 {
	switch class {
	case "small":
		return 20 << 30
	case "xl":
		return 80 << 30
	default:
		return 40 << 30
	}
}

// ValidClass reports whether class is one of the three.
func ValidClass(class string) bool {
	return class == "small" || class == "large" || class == "xl"
}

// HostReserve is the memory kept for the host itself (DESIGN.md §4).
func HostReserve(memBytes int64) int64 {
	if memBytes >= 128<<30 {
		return 16 << 30
	}
	return 8 << 30
}

// Pick is a placement.
type Pick struct {
	HostID    uuid.UUID
	Name      string
	FreeBytes int64
}

// PickHost chooses a host inside tx and takes the placement lock; the
// caller must record the placement (projects.host_id) in the same
// transaction so the next placement sees it.
func PickHost(ctx context.Context, tx pgx.Tx, class string, volumeBytes int64, now time.Time) (Pick, error) {
	if _, err := tx.Exec(ctx, "select pg_advisory_xact_lock($1)", LockPlacement); err != nil {
		return Pick{}, err
	}
	need := ClassRAM(class)
	var p Pick
	err := tx.QueryRow(ctx, `
		select h.id, h.name,
		       least(h.free_mem_bytes, h.mem_bytes - (case when h.mem_bytes >= (128::bigint<<30) then 16::bigint<<30 else 8::bigint<<30 end) - coalesce(r.reserved_bytes, 0)) as free
		  from hosts h
		  left join host_reservations r on r.host_id = h.id
		 where h.state = 'ready'
		   and not h.draining
		   and h.last_heartbeat_at is not null
		   and h.last_heartbeat_at > $1
		   and h.pool_free_bytes >= $2
		   and least(h.free_mem_bytes, h.mem_bytes - (case when h.mem_bytes >= (128::bigint<<30) then 16::bigint<<30 else 8::bigint<<30 end) - coalesce(r.reserved_bytes, 0)) >= $3
		 order by free desc, h.name
		 limit 1`, now.Add(-HeartbeatWindow), volumeBytes, need).Scan(&p.HostID, &p.Name, &p.FreeBytes)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Pick{}, ErrNoCapacity
		}
		return Pick{}, err
	}
	return p, nil
}
