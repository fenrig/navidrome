package scanner

import (
	"context"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/navidrome/navidrome/core/auth"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
)

// ScanMissingBPM scans all library tracks that are missing BPM values and
// persists any newly detected BPM values.
func ScanMissingBPM(ctx context.Context, ds model.DataStore) error {
	release, err := lockScan(ctx)
	if err != nil {
		return err
	}
	defer release()

	ctx = auth.WithAdminUser(ctx, ds)
	log.Info(ctx, "Scanner: Starting BPM backfill")

	phase := &phaseFolders{ctx: ctx, ds: ds}
	cursor, err := ds.MediaFile(ctx).GetCursor(model.QueryOptions{
		Filters: squirrel.And{
			squirrel.Eq{"missing": false},
			squirrel.Eq{"bpm": nil},
		},
		Sort:  "id",
		Order: "asc",
	})
	if err != nil {
		return fmt.Errorf("loading tracks for BPM backfill: %w", err)
	}

	var processed, updated, skipped int
	for mf, err := range cursor {
		if err != nil {
			return fmt.Errorf("reading tracks for BPM backfill: %w", err)
		}
		processed++
		lib := model.Library{ID: mf.LibraryID, Path: mf.LibraryPath}
		if lib.Path == "" {
			libPath, err := ds.Library(ctx).GetPath(mf.LibraryID)
			if err != nil {
				log.Warn(ctx, "Scanner: Error resolving library path for BPM backfill", "track", mf.Path, "libraryID", mf.LibraryID, err)
				skipped++
				continue
			}
			lib.Path = libPath
		}

		phase.analyzeBPM(&mf, lib)
		if mf.BPM == nil {
			skipped++
			continue
		}
		if err := ds.MediaFile(ctx).Put(&mf); err != nil {
			log.Error(ctx, "Scanner: Error persisting BPM backfill result", "track", mf.Path, "libraryID", mf.LibraryID, err)
			skipped++
			continue
		}
		updated++
	}

	log.Info(ctx, "Scanner: Finished BPM backfill", "processed", processed, "updated", updated, "skipped", skipped)
	return nil
}
