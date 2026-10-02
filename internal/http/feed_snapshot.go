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

// A store that does not answer quickly is treated as missing.
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
	readCtx, cancel := context.WithTimeout(ctx, feedSnapshotTimeout)
	data, generation, err := s.snapshots.ReadObject(readCtx, feedSnapshotObject)
	cancel()
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
	writeCtx, cancel := context.WithTimeout(ctx, feedSnapshotTimeout)
	defer cancel()
	if err := s.putFeedSnapshot(writeCtx, built, rows, generation); err != nil && !errors.Is(err, storage.ErrGenerationMismatch) {
		s.logger.Warn("feed snapshot write failed", "error", err.Error())
	}
	return rows, "db", nil
}

// refreshFeedSnapshot stores a fresh copy after a write that changes the feed
// has committed. Like snapshotFeed it finishes before the response, and it
// carries on if the client hangs up. Every attempt reads the generation before
// it queries, so a copy built from older rows can never replace a newer one.
// If no attempt lands, the copy is deleted and the next reader rebuilds it.
func (s *Server) refreshFeedSnapshot(ctx context.Context) {
	if s.snapshots == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		var generation int64
		if _, generation, err = s.snapshots.ReadObject(ctx, feedSnapshotObject); err != nil {
			break
		}
		built := time.Now().UTC()
		var rows []feedRow
		if rows, err = s.queryFeed(ctx, "", "", ""); err != nil {
			break
		}
		if err = s.putFeedSnapshot(ctx, built, rows, generation); !errors.Is(err, storage.ErrGenerationMismatch) {
			break
		}
	}
	if err == nil {
		return
	}
	s.logger.Warn("feed snapshot refresh failed, deleting it", "error", err.Error())
	if err := s.snapshots.DeleteObject(ctx, feedSnapshotObject); err != nil {
		s.logger.Error("feed snapshot delete failed", "error", err.Error())
	}
}

func (s *Server) putFeedSnapshot(ctx context.Context, built time.Time, rows []feedRow, generation int64) error {
	data, err := json.Marshal(feedSnapshot{BuiltAt: built, Items: rows})
	if err != nil {
		return err
	}
	return s.snapshots.WriteObject(ctx, feedSnapshotObject, "application/json", data, generation)
}
