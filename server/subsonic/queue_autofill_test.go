package subsonic

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
	"github.com/navidrome/navidrome/tests"
)

func TestAutofillPlayQueueAppendsRecommendations(t *testing.T) {
	user := model.User{ID: "u1", UserName: "user"}
	userRepo := tests.CreateMockUserRepo()
	if err := userRepo.Put(&user); err != nil {
		t.Fatalf("put user: %v", err)
	}

	queueRepo := &tests.MockPlayQueueRepo{
		Queue: &model.PlayQueue{
			UserID:  user.ID,
			Current: 1,
			Items: model.MediaFiles{
				testQueueTrack("seed", "Seed", "Artist A", "Album A", "Genre X", 140, true),
				testQueueTrack("existing", "Existing", "Artist Z", "Album Z", "Genre Z", 90, false),
			},
		},
	}
	mediaRepo := tests.CreateMockMediaFileRepo()
	mediaRepo.SetData(model.MediaFiles{
		testQueueTrack("seed", "Seed", "Artist A", "Album A", "Genre X", 140, true),
		testQueueTrack("recommended", "Recommended", "Artist A", "Album B", "Genre X", 141, false),
		testQueueTrack("unrelated", "Unrelated", "Artist Q", "Album Q", "Genre Y", 90, false),
	})

	api := &Router{
		ds: &tests.MockDataStore{
			MockedUser:      userRepo,
			MockedPlayQueue: queueRepo,
			MockedMediaFile: mediaRepo,
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/rest/getPlayQueueByIndex", nil)
	req = req.WithContext(request.WithUser(req.Context(), user))

	pq, err := api.autofillPlayQueue(req, queueRepo.Queue)
	if err != nil {
		t.Fatalf("autofill: %v", err)
	}
	if pq == nil {
		t.Fatal("expected queue")
	}
	if got := len(pq.Items); got != 3 {
		t.Fatalf("expected 3 items, got %d", got)
	}
	if pq.Items[2].ID != "recommended" {
		t.Fatalf("expected recommended track appended, got %s", pq.Items[2].ID)
	}
	if queueRepo.Queue == nil || len(queueRepo.Queue.Items) != 3 {
		t.Fatalf("expected persisted queue to be updated, got %#v", queueRepo.Queue)
	}
}

func testQueueTrack(id, title, artist, album, genre string, bpm int, starred bool) model.MediaFile {
	return model.MediaFile{
		ID:          id,
		Title:       title,
		Artist:      artist,
		ArtistID:    artist,
		Album:       album,
		AlbumID:     album,
		AlbumArtist: artist,
		Genres:      model.Genres{{Name: genre}},
		Tags:        model.Tags{model.TagGenre: []string{genre}},
		BPM:         intPtr(bpm),
		Starred:     starred,
	}
}

func intPtr(v int) *int {
	return &v
}
