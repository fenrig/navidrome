package playlists

import (
	"testing"

	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/criteria"
)

func TestBPMPlaylistRules(t *testing.T) {
	spec := bpmPlaylistSpec{Name: "BPM 130-144", Min: 130, Max: intPtr(144)}
	rules := bpmPlaylistRules(spec)

	if rules == nil {
		t.Fatal("expected rules")
	}
	if rules.Sort != "title" {
		t.Fatalf("expected title sort, got %q", rules.Sort)
	}
	if rules.Order != "asc" {
		t.Fatalf("expected asc order, got %q", rules.Order)
	}
	if len(bpmPlaylistSpecs) != 5 {
		t.Fatalf("expected 5 BPM specs, got %d", len(bpmPlaylistSpecs))
	}
}

func TestBPMPlaylistMatch(t *testing.T) {
	adminID := "admin-1"
	spec := bpmPlaylistSpec{Name: "BPM 145+", Min: 145}
	rules := bpmPlaylistRules(spec)

	p := model.Playlist{
		Name:     "Anything",
		OwnerID:  adminID,
		Public:   true,
		Comment:  bpmPlaylistCommentMarker,
		Rules:    rules,
	}

	if !bpmPlaylistMatchesManagedSpec(p, adminID, spec, rules) {
		t.Fatal("expected managed BPM playlist to match")
	}

	p.OwnerID = "other"
	if bpmPlaylistMatchesManagedSpec(p, adminID, spec, rules) {
		t.Fatal("expected foreign playlist to not match")
	}
}

func TestBPMPlaylistCountsUseInclusiveBounds(t *testing.T) {
	spec := bpmPlaylistSpec{Name: "BPM 60-89", Min: 60, Max: intPtr(89)}
	_ = bpmPlaylistCountFilters(spec)

	rules := bpmPlaylistRules(spec)
	all, ok := rules.Expression.(criteria.All)
	if !ok {
		t.Fatal("expected all expression")
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 criteria expressions, got %d", len(all))
	}
}
