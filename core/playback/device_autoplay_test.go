package playback

import (
	"context"
	"testing"

	"github.com/navidrome/navidrome/model"
)

func TestExtendQueueForAutoplayAppendsTracks(t *testing.T) {
	dev := &playbackDevice{
		serviceCtx: context.Background(),
		PlaybackQueue: &Queue{
			Index: 1,
			Items: model.MediaFiles{
				{ID: "seed", Title: "Seed", Artist: "Artist A", ArtistID: "Artist A", AlbumID: "Album A"},
				{ID: "existing", Title: "Existing", Artist: "Artist Z", ArtistID: "Artist Z", AlbumID: "Album Z"},
			},
		},
		ParentPlaybackServer: &fakeAutoplayServer{
			recommended: model.MediaFiles{
				{ID: "next", Title: "Next", Artist: "Artist A", ArtistID: "Artist A", AlbumID: "Album B"},
			},
		},
	}

	if err := dev.extendQueueForAutoplay(); err != nil {
		t.Fatalf("extend autoplay: %v", err)
	}
	if got := len(dev.PlaybackQueue.Items); got != 3 {
		t.Fatalf("expected 3 tracks, got %d", got)
	}
	if dev.PlaybackQueue.Items[2].ID != "next" {
		t.Fatalf("expected next track appended, got %s", dev.PlaybackQueue.Items[2].ID)
	}
}

type fakeAutoplayServer struct {
	recommended model.MediaFiles
}

func (f *fakeAutoplayServer) Run(context.Context) error { return nil }

func (f *fakeAutoplayServer) GetDeviceForUser(string) (*playbackDevice, error) { return nil, nil }

func (f *fakeAutoplayServer) GetMediaFile(id string) (*model.MediaFile, error) {
	for i := range f.recommended {
		if f.recommended[i].ID == id {
			return &f.recommended[i], nil
		}
	}
	return nil, model.ErrNotFound
}

func (f *fakeAutoplayServer) RecommendTracks(_ context.Context, _ model.MediaFiles, _ []string, _ int) (model.MediaFiles, error) {
	return f.recommended, nil
}
