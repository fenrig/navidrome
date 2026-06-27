package playlists

import (
	"slices"
	"strings"

	"github.com/navidrome/navidrome/model"
)

type TrackAffinity struct{}

func NewTrackAffinity() *TrackAffinity {
	return &TrackAffinity{}
}

func (a *TrackAffinity) Score(candidate model.MediaFile, seeds, recent model.MediaFiles) float64 {
	return discoveryCandidateScore(candidate, seeds, recent)
}

func (a *TrackAffinity) Recommend(pool, context model.MediaFiles, count int) model.MediaFiles {
	if count <= 0 || len(pool) == 0 {
		return nil
	}
	if count > len(pool) {
		count = len(pool)
	}

	seeds, hasFavorites := discoverySeeds(context, min(12, len(context)))
	if !hasFavorites {
		return buildPopularityMix(pool, discoveryPlaylistSpec{
			Name:         "Autofill",
			MaxTracks:    count,
			SeedTracks:   0,
			MaxPerArtist: count,
			MaxPerAlbum:  count,
			MaxPerLabel:  count,
		})
	}

	type scoredTrack struct {
		track model.MediaFile
		score float64
	}

	scored := make([]scoredTrack, 0, len(pool))
	for _, candidate := range pool {
		score := discoveryCandidateScore(candidate, seeds, context)
		if score <= 0 {
			continue
		}
		scored = append(scored, scoredTrack{track: candidate, score: score})
	}

	slices.SortStableFunc(scored, func(a, b scoredTrack) int {
		if cmp := compareFloatDesc(a.score, b.score); cmp != 0 {
			return cmp
		}
		if cmp := compareBoolDesc(a.track.Starred, b.track.Starred); cmp != 0 {
			return cmp
		}
		if cmp := compareIntDesc(a.track.Rating, b.track.Rating); cmp != 0 {
			return cmp
		}
		if cmp := compareInt64Desc(a.track.PlayCount, b.track.PlayCount); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.track.Title, b.track.Title)
	})

	if len(scored) == 0 {
		return buildPopularityMix(pool, discoveryPlaylistSpec{
			Name:         "Autofill",
			MaxTracks:    count,
			SeedTracks:   0,
			MaxPerArtist: count,
			MaxPerAlbum:  count,
			MaxPerLabel:  count,
		})
	}

	out := make(model.MediaFiles, 0, count)
	seen := map[string]struct{}{}
	for _, item := range scored {
		if len(out) >= count {
			break
		}
		if _, ok := seen[item.track.ID]; ok {
			continue
		}
		out = append(out, item.track)
		seen[item.track.ID] = struct{}{}
	}
	return out
}
