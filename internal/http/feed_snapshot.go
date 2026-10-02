package http

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"yalinli-api/internal/storage"
)

// The homepage asks for the unfiltered feed on every render, and most renders
// are bots. Each one used to be a query, and the managed Postgres behind this
// wakes for any query and is billed for at least five minutes after it. So the
// unfiltered feed is kept as a private object next to the uploads: readers are
// served from it without touching the database, and every write through this
// API that changes the feed rebuilds it before responding. The copy holds
// storage refs and no name for anonymous entries; URLs are signed and labels
// are worked out per request, as before.
//
// Moderation done by hand in SQL does not pass through here, so a hidden or
// deleted entry stays on the homepage until the copy is older than
// FEED_SNAPSHOT_MAX_AGE. Deleting the object brings that forward:
//
//	gcloud storage rm gs://yalinli-images-a1b2c3/_cache/feed-v1.json --project yalinli
const feedSnapshotObject = "_cache/feed-v1.json"

// Each read or write of the copy gets this long; a store that does not answer
// in time is treated as missing.
const feedSnapshotTimeout = 3 * time.Second

type feedSnapshot struct {
	BuiltAt time.Time `json:"builtAt"`
	Items   []feedRow `json:"items"`
}

// snapshotFeed serves the unfiltered feed from the stored copy while it is
// younger than FEED_SNAPSHOT_MAX_AGE. Otherwise it queries the database and
// stores the result for the next reader before responding, because Cloud Run
// throttles the CPU once the response is out.
func (s *Server) snapshotFeed(ctx context.Context) ([]feedRow, string, error) {
	data, generation, err := s.readFeedSnapshot(ctx)
	if err != nil {
		s.logger.Warn("feed snapshot read failed", "error", err.Error())
		rows, err := s.queryFeed(ctx, "", "", "")
		return rows, "db", err
	}
	var snap feedSnapshot
	stored := generation != 0 && json.Unmarshal(data, &snap) == nil && snap.Items != nil
	if stored && time.Since(snap.BuiltAt) < s.cfg.FeedSnapshotMaxAge {
		return snap.Items, "snapshot", nil
	}
	built := time.Now().UTC()
	rows, err := s.queryFeed(ctx, "", "", "")
	if err != nil {
		if stored {
			s.logger.Warn("feed query failed, serving the expired snapshot", "error", err.Error())
			return snap.Items, "expired_snapshot", nil
		}
		return nil, "db", err
	}
	if err := s.putFeedSnapshot(ctx, built, rows, generation); err != nil && !errors.Is(err, storage.ErrGenerationMismatch) {
		s.logger.Warn("feed snapshot write failed", "error", err.Error())
	}
	return rows, "db", nil
}

// refreshFeedSnapshot stores a fresh copy after a write that changes the feed
// has committed. Like snapshotFeed it finishes before the response, and it
// carries on if the client hangs up. Every attempt reads the generation before
// it queries, so a copy built from older rows can never replace a newer one.
// If no attempt lands, the copy is deleted and the next reader rebuilds it.
//
// One gap is left: with no copy at all, a reader that queried before this
// write committed can still create one after this refresh failed, and that
// copy lacks the write until it expires. It takes a missing copy, a reader in
// that second or two and a failed write at once.
func (s *Server) refreshFeedSnapshot(ctx context.Context) {
	if s.snapshots == nil {
		return
	}
	ctx = context.WithoutCancel(ctx)
	attempts, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		var generation int64
		if _, generation, err = s.readFeedSnapshot(attempts); err != nil {
			break
		}
		built := time.Now().UTC()
		var rows []feedRow
		if rows, err = s.queryFeed(attempts, "", "", ""); err != nil {
			break
		}
		if err = s.putFeedSnapshot(attempts, built, rows, generation); !errors.Is(err, storage.ErrGenerationMismatch) {
			break
		}
	}
	if err == nil {
		return
	}
	s.logger.Warn("feed snapshot refresh failed, deleting it", "error", err.Error())
	// The attempts may have used up their time, so the delete gets its own.
	delCtx, cancelDel := context.WithTimeout(ctx, feedSnapshotTimeout)
	defer cancelDel()
	if err := s.snapshots.DeleteObject(delCtx, feedSnapshotObject); err != nil {
		s.logger.Error("feed snapshot delete failed, the homepage may miss this write until the copy expires", "error", err.Error(), "fix", "gcloud storage rm gs://"+s.cfg.GCSBucket+"/"+feedSnapshotObject+" --project yalinli")
	}
}

func (s *Server) readFeedSnapshot(ctx context.Context) ([]byte, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, feedSnapshotTimeout)
	defer cancel()
	return s.snapshots.ReadObject(ctx, feedSnapshotObject)
}

func (s *Server) putFeedSnapshot(ctx context.Context, built time.Time, rows []feedRow, generation int64) error {
	data, err := json.Marshal(feedSnapshot{BuiltAt: built, Items: rows})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, feedSnapshotTimeout)
	defer cancel()
	return s.snapshots.WriteObject(ctx, feedSnapshotObject, "application/json", data, generation)
}
