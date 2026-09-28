package billing

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/heracraft/repose/internal/api/ops"
	"github.com/heracraft/repose/internal/api/store"
)

// ErrDisabled is returned by every Paddle-backed call while billing is not
// configured (DECISIONS I-16, I-289). The HTTP layer maps it to
// `503 billing_disabled`.
var ErrDisabled = errors.New("billing_disabled")

// Stopper enqueues the stop of a project's guest. The ops engine satisfies
// it; a test can substitute a recorder.
type Stopper interface {
	Enqueue(ctx context.Context, q store.Querier, n ops.NewOp, allowQueue bool) (uuid.UUID, error)
	Kick()
}

// EventSink records a platform event on a project so the notification
// outbox delivers it. events.Ingest.Platform satisfies it.
type EventSink interface {
	Platform(ctx context.Context, projectID uuid.UUID, kind, summary string) error
}

// ChargeSender is the one Paddle call the overage job makes; the client
// satisfies it and tests record it.
type ChargeSender interface {
	CreateOneTimeCharge(ctx context.Context, subscriptionID string, cents int64, description, effectiveFrom string) (transactionID string, err error)
}
