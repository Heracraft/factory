package guest

import (
	"context"
	"fmt"
	"io"
	"path"
	"time"

	hostdv1 "github.com/heracraft/repose/internal/gen/hostd/v1"
	"github.com/heracraft/repose/internal/hostd/snapshot"
	"github.com/heracraft/repose/internal/hostd/state"
)

// FreezeWarnAfter is the freeze window that produces a warning.
const FreezeWarnAfter = 2 * time.Second

func (m *Manager) snapshotCmd(ctx context.Context, c *hostdv1.Snapshot) (*hostdv1.SnapshotResult, *Error) {
	g, gerr := m.getGuest(c.GuestId)
	if gerr != nil {
		return nil, gerr
	}
	switch c.Reason {
	case "scheduled", "stop", "manual":
	case "":
		c.Reason = "manual"
	default:
		return nil, errf(CodeInvalidArgument, "reason %q must be scheduled, stop or manual", c.Reason)
	}
	return m.snapshotGuest(ctx, g, c.Reason)
}

// BlobPath is <user_id>/<project_id>/<ts>.img.zst; a guest created without
// a user_id (an old CreateGuest shape) falls under its project only.
func BlobPath(g *state.Guest, ts time.Time) string {
	name := ts.UTC().Format("2006-01-02T15-04-05.000Z") + ".img.zst"
	if g.UserID == "" {
		return path.Join(g.ProjectID, name)
	}
	return path.Join(g.UserID, g.ProjectID, name)
}

// snapshotGuest freezes (when running), takes the LVM snapshot, thaws,
// streams it to the store and removes the LVM snapshot whatever happened.
func (m *Manager) snapshotGuest(ctx context.Context, g *state.Guest, reason string) (*hostdv1.SnapshotResult, *Error) {
	switch g.State {
	case StateRunning, StateStopped, StateError:
	default:
		return nil, errf(CodeInvalidArgument, "guest is %s; snapshot needs running or stopped", g.State)
	}
	start := m.d.Now()
	ts := start
	snap := fmt.Sprintf("snap-%s-%d", g.GuestID, ts.UnixMilli())
	vol := VolumeName(g.GuestID)
	log := m.log(g).With("reason", reason)
	log.Info("snapshot start", "event", "snapshot_start")
	result := "ok"
	defer func() {
		if m.d.Metrics != nil {
			m.d.Metrics.SnapshotDuration.WithLabelValues(reason, result).Observe(m.d.Now().Sub(start).Seconds())
		}
	}()
	fail := func(e *Error) (*hostdv1.SnapshotResult, *Error) {
		result = e.Code
		log.Error("snapshot failed", "event", "snapshot_fail", "code", e.Code, "reason", e.Message)
		return nil, e
	}

	if g.State == StateRunning {
		sess, serr := m.session(g.GuestID)
		if serr != nil {
			return fail(serr)
		}
		fstart := m.d.Now()
		if err := sess.Freeze(ctx); err != nil {
			return fail(errf(CodeGuestUnresponsive, "guestd Freeze: %v", err))
		}
		lvErr := m.d.LVM.Snapshot(ctx, vol, snap)
		thawErr := sess.Thaw(ctx)
		window := m.d.Now().Sub(fstart)
		if m.d.Metrics != nil {
			m.d.Metrics.SnapshotFreezeSeconds.Observe(window.Seconds())
		}
		if thawErr != nil {
			// guestd's watchdog thaws after 10 s; say so.
			m.Warn("freeze_timeout", "guest "+g.GuestID+": thaw request failed; guestd watchdog will thaw")
			if lvErr == nil {
				_ = m.d.LVM.RemoveVolume(ctx, snap) // the snapshot is unusable without a clean thaw record
			}
			return fail(errf(CodeInternal, "snapshot: thaw failed: %v", thawErr))
		}
		if window > FreezeWarnAfter {
			log.Warn("freeze window long", "event", "snapshot_start", "freeze_ms", window.Milliseconds())
		}
		if lvErr != nil {
			return fail(errf(CodeInternal, "lvcreate -s: %v", lvErr))
		}
	} else {
		if err := m.d.LVM.Snapshot(ctx, vol, snap); err != nil {
			return fail(errf(CodeInternal, "lvcreate -s: %v", err))
		}
	}
	defer func() {
		_ = m.d.LVM.RemoveVolume(context.Background(), snap) // always, even on upload failure; a leftover snapshot is found by reconcile
	}()
	if err := m.d.LVM.Activate(ctx, snap); err != nil {
		return fail(errf(CodeInternal, "lvchange -ay -K: %v", err))
	}
	r, err := m.d.Stream.Read(ctx, m.d.LVM.DevPath(snap))
	if err != nil {
		return fail(errf(CodeInternal, "snapshot stream: %v", err))
	}
	blobPath := BlobPath(g, ts)
	meta := map[string]string{"guest_id": g.GuestID, "class": g.Class, "volume_bytes": fmt.Sprint(g.VolumeBytes), "reason": reason}
	n, uerr := m.d.Blob.Upload(ctx, blobPath, r, meta)
	cerr := r.Close()
	if uerr == nil && cerr != nil {
		uerr = cerr
	}
	if uerr != nil {
		return fail(errf(CodeInternal, "snapshot upload failed: %v", uerr))
	}
	if m.d.Metrics != nil {
		m.d.Metrics.SnapshotBytesTotal.Add(float64(n))
		m.d.Metrics.SnapshotBytes.Set(float64(n))
	}
	// The format and the used bytes say what the duration was spent on:
	// an extent snapshot reads what the filesystem uses, a raw one reads
	// the whole volume (I-164).
	format, why, used := "raw", "", uint64(0)
	if md, ok := r.(snapshot.Moder); ok {
		mo := md.Mode()
		format, why, used = mo.Format, mo.Why, mo.UsedBytes
	}
	log.Info("snapshot done", "event", "snapshot_done", "bytes", n, "duration_ms", m.d.Now().Sub(start).Milliseconds(),
		"format", format, "raw_reason", why, "used_bytes", used, "volume_bytes", g.VolumeBytes)
	m.emitEvent(&hostdv1.Event_SnapshotDone{SnapshotDone: &hostdv1.SnapshotDone{GuestId: g.GuestID, BlobPath: blobPath, Bytes: n}})
	return &hostdv1.SnapshotResult{BlobPath: blobPath, Bytes: n}, nil
}

func (m *Manager) restore(ctx context.Context, c *hostdv1.Restore) (*hostdv1.CreateResult, *Error) {
	if c.GuestId == "" || c.ProjectId == "" || c.BlobPath == "" {
		return nil, errf(CodeInvalidArgument, "guest_id, project_id and blob_path required")
	}
	if _, err := hexPrefix(c.GuestId); err != nil {
		return nil, err.(*Error)
	}
	existing, gerr := m.getGuest(c.GuestId)
	if gerr != nil && gerr.Code != CodeNotFound {
		return nil, gerr
	}
	if existing != nil && existing.State != StateRestoring && existing.State != StateError {
		if existing.State == StateStopped {
			return &hostdv1.CreateResult{GuestIp: existing.IP, VsockCid: existing.CID}, nil
		}
		return nil, errf(CodeAlreadyExists, "guest %s exists in state %s", c.GuestId, existing.State)
	}
	if err := m.capacityCheck(c.Class, c.VolumeBytes, false); err != nil {
		return nil, err
	}
	if ok, err := m.d.Blob.Exists(ctx, c.BlobPath); err != nil {
		return nil, errf(CodeInternal, "blob: %v", err)
	} else if !ok {
		return nil, errf(CodeNotFound, "snapshot %s not in the store", c.BlobPath)
	}
	if c.SystemClosure != "" {
		if ok, err := m.d.Nix.PathExists(ctx, c.SystemClosure); err != nil {
			return nil, errf(CodeInternal, "nix path-info: %v", err)
		} else if !ok {
			return nil, errf(CodeNotFound, "system closure %s is not in the host store", c.SystemClosure)
		}
	}
	idx, err := m.d.State.AllocIndex(c.GuestId, m.maxIndex)
	if err != nil {
		return nil, errf(CodeInsufficientCapacity, "%v", err)
	}
	tap, _ := TapName(c.GuestId)
	mac, _ := MACAddr(c.GuestId)
	g := &state.Guest{
		GuestID: c.GuestId, ProjectID: c.ProjectId, UserID: c.UserId, ProjectSlug: c.ProjectSlug, RemoteURL: c.RemoteUrl,
		Class: c.Class, VolumeBytes: c.VolumeBytes, SystemClosure: c.SystemClosure,
		IP: m.ipForIndex(idx), MAC: mac, Tap: tap, CID: cidForIndex(idx), IPIndex: idx,
		Env: c.Env, Principals: c.Principals, SSHCAPub: c.SshCaPub, HooksConfig: c.HooksConfig, ProjectJSON: c.ProjectJson,
	}
	if err := m.setState(g, StateRestoring, ""); err != nil {
		return nil, errf(CodeInternal, "%v", err)
	}
	m.log(g).Info("restoring guest", "event", "guest_create", "class", g.Class)
	fail := func(e *Error) (*hostdv1.CreateResult, *Error) {
		_ = m.setState(g, StateError, "restore: "+e.Message) // e is what the api sees; the state write is logged inside
		return nil, e
	}
	vol := VolumeName(g.GuestID)
	if err := m.d.LVM.CreateVolume(ctx, vol, g.VolumeBytes); err != nil {
		return fail(errf(CodeInternal, "lvcreate: %v", err))
	}
	pr, pw := io.Pipe()
	dlErr := make(chan error, 1)
	go func() {
		err := m.d.Blob.Download(ctx, c.BlobPath, pw)
		_ = pw.CloseWithError(err) // the reader sees err; CloseWithError itself cannot fail
		dlErr <- err
	}()
	werr := m.d.Stream.Write(ctx, m.d.LVM.DevPath(vol), pr)
	_ = pr.Close() // stops the writer if it is still going; the error is the download's
	if err := <-dlErr; err != nil {
		return fail(errf(CodeInternal, "snapshot download failed: %v", err))
	}
	if werr != nil {
		return fail(errf(CodeInternal, "restore stream: %v", werr))
	}
	code, err := m.d.LVM.Fsck(ctx, vol)
	if err != nil {
		return fail(errf(CodeInternal, "e2fsck: %v", err))
	}
	if code > 1 {
		return fail(errf(CodeInternal, "filesystem check failed after restore (e2fsck exit %d)", code))
	}
	if g.SystemClosure != "" {
		if err := m.d.Roots.Set(g.GuestID, g.SystemClosure); err != nil {
			return fail(errf(CodeInternal, "gcroot: %v", err))
		}
	}
	if err := m.cacheSecrets(g.GuestID, c.Secrets, c.SshCaPub, c.HostKey, c.HostCert); err != nil {
		return fail(err)
	}
	if err := m.setState(g, StateStopped, ""); err != nil {
		return nil, errf(CodeInternal, "%v", err)
	}
	return &hostdv1.CreateResult{GuestIp: g.IP, VsockCid: g.CID}, nil
}
