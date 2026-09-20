// Package snapshots owns retention (05-control-plane-api.md §5.8,
// docs/features/snapshots.md): live projects keep seven days of
// snapshots and always their newest one; destroyed projects keep the
// last snapshot until expires_at; a snapshot a restore is reading is
// never touched. Blobs are deleted through the BlobStore, then the row
// gets deleted_at.
package snapshots

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
	"github.com/google/uuid"

	"github.com/heracraft/repose/internal/api/metrics"
	"github.com/heracraft/repose/internal/db"
)

// BlobStore deletes snapshot blobs. Deleting a missing blob is not an
// error.
type BlobStore interface {
	Delete(ctx context.Context, path string) error
}

// Retention is the live-project window.
const Retention = 7 * 24 * time.Hour

// Expiry is the job.
type Expiry struct {
	pool *db.Pool
	blob BlobStore
	m    *metrics.M
	log  *slog.Logger
	Now  func() time.Time
}

// New builds the job.
func New(pool *db.Pool, blob BlobStore, m *metrics.M, log *slog.Logger) *Expiry {
	return &Expiry{pool: pool, blob: blob, m: m, log: log.With("component", "api"), Now: time.Now}
}

// Run executes the job daily until ctx ends.
func (e *Expiry) Run(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		release, ok, err := db.TryLock(ctx, e.pool, db.LockSnapshots)
		if err == nil && ok {
			if _, err := e.Once(ctx); err != nil && ctx.Err() == nil {
				e.log.Error("snapshot expiry", "event", "snapshot_expiry_fail", "err", err.Error())
			}
			release()
		}
		e.UpdateAgeGauge(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Once deletes every expired snapshot and returns the ids deleted.
func (e *Expiry) Once(ctx context.Context) ([]uuid.UUID, error) {
	now := e.Now()
	var deleted []uuid.UUID
	for {
		var id uuid.UUID
		var path string
		// One row per transaction: locked, checked, deleted from Blob, then
		// marked; a restore holding the row (restoring_op_id) is skipped.
		err := db.InTx(ctx, e.pool, func(tx db.Tx) error {
			err := tx.QueryRow(ctx, `select s.id, s.blob_path from snapshots s join projects p on p.id = s.project_id
				where s.deleted_at is null and s.restoring_op_id is null and s.id not in (select id from snapshots x where x.deleted_at is null and x.restoring_op_id is not null)
				  and (
				    (p.destroyed_at is not null and s.expires_at is not null and s.expires_at < $1)
				    or (p.destroyed_at is null and s.taken_at < $2 and s.id <> (select id from snapshots n where n.project_id = s.project_id and n.deleted_at is null order by n.taken_at desc, n.created_at desc limit 1))
				  )
				order by s.taken_at limit 1 for update of s skip locked`, now, now.Add(-Retention)).Scan(&id, &path)
			if err != nil {
				if db.IsNoRows(err) {
					return errDone
				}
				return err
			}
			if err := e.blob.Delete(ctx, path); err != nil {
				return fmt.Errorf("delete blob: %w", err)
			}
			_, err = tx.Exec(ctx, "update snapshots set deleted_at = $2 where id = $1", id, now)
			return err
		})
		if errors.Is(err, errDone) {
			return deleted, nil
		}
		if err != nil {
			return deleted, err
		}
		deleted = append(deleted, id)
		e.log.Info("snapshot expired", "event", "snapshot_expired", "snapshot_id", id.String())
	}
}

var errDone = errors.New("no more expired snapshots")

// UpdateAgeGauge sets repose_api_snapshot_age_seconds to the oldest
// newest-snapshot age over running projects (the SnapshotStale input).
func (e *Expiry) UpdateAgeGauge(ctx context.Context) {
	var oldest *time.Time
	err := e.pool.QueryRow(ctx, `select min(newest) from (select (select max(taken_at) from snapshots s where s.project_id = p.id and s.deleted_at is null) as newest from projects p where p.state = 'running' and p.destroyed_at is null) t`).Scan(&oldest)
	if err != nil || oldest == nil {
		e.m.SnapshotAgeSeconds.Set(0)
		return
	}
	e.m.SnapshotAgeSeconds.Set(e.Now().Sub(*oldest).Seconds())
}

// AzureBlob deletes from the repose-snapshots container.
type AzureBlob struct {
	client    *azblob.Client
	container string
}

// NewAzureBlob connects with the environment's identity; accountURL is
// https://<account>.blob.core.windows.net.
func NewAzureBlob(accountURL, container string, cred azcore.TokenCredential) (*AzureBlob, error) {
	if _, err := url.Parse(accountURL); err != nil || !strings.HasPrefix(accountURL, "https://") {
		return nil, fmt.Errorf("blob account url %q", accountURL)
	}
	if cred == nil {
		c, err := azidentity.NewDefaultAzureCredential(nil)
		if err != nil {
			return nil, fmt.Errorf("azure credential: %w", err)
		}
		cred = c
	}
	client, err := azblob.NewClient(accountURL, cred, nil)
	if err != nil {
		return nil, err
	}
	if container == "" {
		container = "repose-snapshots"
	}
	return &AzureBlob{client: client, container: container}, nil
}

// Delete implements BlobStore.
func (a *AzureBlob) Delete(ctx context.Context, path string) error {
	_, err := a.client.DeleteBlob(ctx, a.container, path, nil)
	if err != nil && bloberror.HasCode(err, bloberror.BlobNotFound) {
		return nil
	}
	return err
}
