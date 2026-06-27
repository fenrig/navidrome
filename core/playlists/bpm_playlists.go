package playlists

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	. "github.com/Masterminds/squirrel"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/criteria"
	"github.com/navidrome/navidrome/model/request"
)

const bpmPlaylistCommentMarker = "Auto-generated BPM playlist"

type bpmPlaylistSpec struct {
	Name string
	Min  int
	Max  *int
}

var bpmPlaylistSpecs = []bpmPlaylistSpec{
	{Name: "BPM 60-89", Min: 60, Max: intPtr(89)},
	{Name: "BPM 90-114", Min: 90, Max: intPtr(114)},
	{Name: "BPM 115-129", Min: 115, Max: intPtr(129)},
	{Name: "BPM 130-144", Min: 130, Max: intPtr(144)},
	{Name: "BPM 145+", Min: 145, Max: nil},
}

func intPtr(v int) *int {
	return &v
}

func bpmPlaylistRules(spec bpmPlaylistSpec) *criteria.Criteria {
	expr := criteria.All{
		criteria.Is{"missing": false},
		criteria.Gte{"bpm": spec.Min},
	}
	if spec.Max != nil {
		expr = append(expr, criteria.Lte{"bpm": *spec.Max})
	}
	return &criteria.Criteria{
		Expression: expr,
		Sort:       "title",
		Order:      "asc",
	}
}

func bpmPlaylistMatchesManagedSpec(p model.Playlist, adminID string, spec bpmPlaylistSpec, rules *criteria.Criteria) bool {
	if p.OwnerID != adminID || !p.Public {
		return false
	}
	if p.Comment == bpmPlaylistCommentMarker {
		return true
	}
	return p.Name == spec.Name && reflect.DeepEqual(p.Rules, rules)
}

func bpmPlaylistCountFilters(spec bpmPlaylistSpec) Sqlizer {
	filters := And{
		Eq{"missing": false},
		GtOrEq{"bpm": spec.Min},
	}
	if spec.Max != nil {
		filters = append(filters, LtOrEq{"bpm": *spec.Max})
	}
	return filters
}

func (s *playlists) SyncGeneratedBPMPlaylists(ctx context.Context) error {
	admin, err := s.ds.User(ctx).FindFirstAdmin()
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			log.Debug(ctx, "Skipping BPM playlist generation: no admin user found")
			return nil
		}
		return fmt.Errorf("finding admin user: %w", err)
	}

	ctx = request.WithUser(ctx, *admin)

	existing, err := s.ds.Playlist(ctx).GetAll(model.QueryOptions{Sort: "name"})
	if err != nil {
		return fmt.Errorf("loading playlists: %w", err)
	}

	for _, spec := range bpmPlaylistSpecs {
		rules := bpmPlaylistRules(spec)
		count, err := s.ds.MediaFile(ctx).CountAll(model.QueryOptions{Filters: bpmPlaylistCountFilters(spec)})
		if err != nil {
			log.Error(ctx, "Error counting media files for BPM playlist", "playlist", spec.Name, err)
			continue
		}

		var target *model.Playlist
		for i := range existing {
			if bpmPlaylistMatchesManagedSpec(existing[i], admin.ID, spec, rules) {
				target = &existing[i]
				break
			}
		}

		if target == nil {
			if count == 0 {
				continue
			}
			target = &model.Playlist{
				Name:     spec.Name,
				Comment:  bpmPlaylistCommentMarker,
				OwnerID:  admin.ID,
				Public:   true,
				Rules:    rules,
				EvaluatedAt: nil,
			}
			if err := s.ds.Playlist(ctx).Put(target); err != nil {
				log.Error(ctx, "Error creating BPM playlist", "playlist", spec.Name, err)
				continue
			}
			existing = append(existing, *target)
		} else {
			target.Name = spec.Name
			target.Comment = bpmPlaylistCommentMarker
			target.OwnerID = admin.ID
			target.Public = true
			target.Rules = rules
			target.EvaluatedAt = nil
			if err := s.ds.Playlist(ctx).Put(target); err != nil {
				log.Error(ctx, "Error updating BPM playlist", "playlist", spec.Name, err)
				continue
			}
		}

		if _, err := s.ds.Playlist(ctx).GetWithTracks(target.ID, true, false); err != nil {
			log.Error(ctx, "Error refreshing BPM playlist", "playlist", spec.Name, err)
			continue
		}
		log.Info(ctx, "Synced BPM playlist", "playlist", spec.Name, "count", count)
	}

	return nil
}
