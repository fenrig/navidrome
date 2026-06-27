package playlists

import (
	"context"
	"testing"

	"github.com/navidrome/navidrome/core"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
	"github.com/navidrome/navidrome/tests"
)

func TestBuildDiscoveryMixPrefersSimilarTracks(t *testing.T) {
	tracks := model.MediaFiles{
		testMixTrack("1", "Seed", "Artist A", "Album A", "Genre X", "Label One", 140, 2020, 25, true, 5),
		testMixTrack("2", "Near A", "Artist A", "Album B", "Genre X", "Label One", 142, 2021, 4, false, 0),
		testMixTrack("3", "Near A 2", "Artist A", "Album C", "Genre X", "Label One", 141, 2019, 2, false, 0),
		testMixTrack("4", "Near B", "Artist B", "Album D", "Genre X", "Label Two", 139, 2020, 3, false, 0),
		testMixTrack("5", "Unrelated", "Artist C", "Album E", "Genre Y", "Label Three", 96, 2010, 1, false, 0),
	}

	selected := buildDiscoveryMix(tracks, discoveryPlaylistSpec{
		Name:         "Discovery Mix",
		MaxTracks:    10,
		SeedTracks:   1,
		MaxPerArtist: 2,
		MaxPerAlbum:  2,
		MaxPerLabel:  3,
	})

	if got := len(selected); got != 3 {
		t.Fatalf("expected 3 tracks, got %d", got)
	}
	if selected[0].ID != "1" || selected[1].ID != "2" || selected[2].ID != "4" {
		t.Fatalf("unexpected track order: %#v", []string{selected[0].ID, selected[1].ID, selected[2].ID})
	}
	if countArtist(selected, "Artist A") != 2 {
		t.Fatalf("expected Artist A to appear twice, got %d", countArtist(selected, "Artist A"))
	}
}

func TestBuildDiscoveryMixFallsBackToPopularity(t *testing.T) {
	tracks := model.MediaFiles{
		testMixTrack("1", "One", "Artist A", "Album A", "Genre X", "Label One", 120, 2020, 10, false, 0),
		testMixTrack("2", "Two", "Artist B", "Album B", "Genre Y", "Label Two", 120, 2020, 30, false, 0),
		testMixTrack("3", "Three", "Artist C", "Album C", "Genre Z", "Label Three", 120, 2020, 20, false, 0),
	}

	selected := buildDiscoveryMix(tracks, discoveryPlaylistSpec{
		Name:         "Discovery Mix",
		MaxTracks:    10,
		SeedTracks:   1,
		MaxPerArtist: 2,
		MaxPerAlbum:  2,
		MaxPerLabel:  3,
	})

	if got := len(selected); got != 3 {
		t.Fatalf("expected 3 tracks, got %d", got)
	}
	if selected[0].ID != "2" {
		t.Fatalf("expected most played track first, got %s", selected[0].ID)
	}
}

func TestBuildDiscoveryMixChainsRecentTracks(t *testing.T) {
	tracks := model.MediaFiles{
		testMixTrack("1", "Seed", "Artist Seed", "Album Seed", "Genre X", "Label One", 140, 2020, 30, true, 5),
		testMixTrack("2", "Bridge", "Artist B", "Album B", "Genre X", "Label One", 141, 2020, 5, false, 0),
		testMixTrack("3", "Follow", "Artist C", "Album C", "Genre Z", "Label One", 141, 2020, 5, false, 0),
		testMixTrack("4", "Distractor", "Artist D", "Album D", "Genre X", "Label Two", 120, 2010, 50, false, 0),
	}

	selected := buildDiscoveryMix(tracks, discoveryPlaylistSpec{
		Name:         "Discovery Mix",
		MaxTracks:    4,
		SeedTracks:   1,
		MaxPerArtist: 2,
		MaxPerAlbum:  2,
		MaxPerLabel:  3,
	})

	if got := len(selected); got != 4 {
		t.Fatalf("expected 4 tracks, got %d", got)
	}
	ids := playlistTrackIDs(toPlaylistTracks(selected))
	if ids[0] != "1" || ids[1] != "2" || ids[2] != "3" {
		t.Fatalf("expected chain 1,2,3,... got %v", ids[:3])
	}
}

func TestSyncGeneratedDiscoveryPlaylist(t *testing.T) {
	ctx := context.Background()
	userRepo := tests.CreateMockUserRepo()
	if err := userRepo.Put(&model.User{ID: "admin", UserName: "admin", IsAdmin: true}); err != nil {
		t.Fatalf("put admin: %v", err)
	}

	trackRepo := tests.CreateMockMediaFileRepo()
	trackRepo.SetData(model.MediaFiles{
		testMixTrack("1", "Seed", "Artist A", "Album A", "Genre X", "Label One", 140, 2020, 25, true, 5),
		testMixTrack("2", "Near A", "Artist A", "Album B", "Genre X", "Label One", 142, 2021, 4, false, 0),
		testMixTrack("3", "Near A 2", "Artist A", "Album C", "Genre X", "Label One", 141, 2019, 2, false, 0),
		testMixTrack("4", "Near B", "Artist B", "Album D", "Genre X", "Label Two", 139, 2020, 3, false, 0),
	})

	playlistRepo := tests.CreateMockPlaylistRepo()
	ds := &tests.MockDataStore{
		MockedUser:      userRepo,
		MockedMediaFile: trackRepo,
		MockedPlaylist:  playlistRepo,
	}
	ps := NewPlaylists(ds, core.NewImageUploadService())

	ctx = request.WithUser(ctx, model.User{ID: "admin", UserName: "admin", IsAdmin: true})
	if err := ps.SyncGeneratedDiscoveryPlaylist(ctx); err != nil {
		t.Fatalf("sync discovery playlist: %v", err)
	}

	if playlistRepo.Last == nil {
		t.Fatal("expected a playlist to be created")
	}
	if playlistRepo.Last.Comment != discoveryPlaylistCommentMarker {
		t.Fatalf("unexpected comment: %q", playlistRepo.Last.Comment)
	}
	if !playlistRepo.Last.Public || playlistRepo.Last.OwnerID != "admin" {
		t.Fatalf("playlist ownership/publicity not set correctly: %#v", playlistRepo.Last)
	}
	if got := len(playlistRepo.Last.Tracks); got != 3 {
		t.Fatalf("expected 3 tracks, got %d", got)
	}
	if ids := playlistTrackIDs(playlistRepo.Last.Tracks); ids[0] != "1" || ids[1] != "2" || ids[2] != "4" {
		t.Fatalf("unexpected track ids: %v", ids)
	}
}

func testMixTrack(id, title, artist, album, genre, label string, bpm, year int, playCount int64, starred bool, rating int) model.MediaFile {
	return model.MediaFile{
		ID:          id,
		Title:       title,
		Artist:      artist,
		ArtistID:    artist,
		Album:       album,
		AlbumID:     album,
		AlbumArtist: artist,
		Genres:      model.Genres{{Name: genre}},
		Tags: model.Tags{
			model.TagGenre:       []string{genre},
			model.TagRecordLabel: []string{label},
		},
		BPM:         intPtr(bpm),
		Year:        year,
		ReleaseYear: year,
		PlayCount:   playCount,
		Starred:     starred,
		Rating:      rating,
		Participants: model.Participants{
			model.RoleArtist:      []model.Participant{{Artist: model.Artist{ID: artist, Name: artist}}},
			model.RoleAlbumArtist: []model.Participant{{Artist: model.Artist{ID: artist, Name: artist}}},
		},
	}
}

func playlistTrackIDs(tracks model.PlaylistTracks) []string {
	ids := make([]string, len(tracks))
	for i := range tracks {
		ids[i] = tracks[i].MediaFileID
	}
	return ids
}

func toPlaylistTracks(tracks model.MediaFiles) model.PlaylistTracks {
	res := make(model.PlaylistTracks, len(tracks))
	for i := range tracks {
		res[i] = model.PlaylistTrack{
			MediaFileID: tracks[i].ID,
			MediaFile:   tracks[i],
		}
	}
	return res
}

func countArtist(tracks model.MediaFiles, artist string) int {
	count := 0
	for _, track := range tracks {
		if track.Artist == artist {
			count++
		}
	}
	return count
}
