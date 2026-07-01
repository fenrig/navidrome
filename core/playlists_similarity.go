package core

import (
	"github.com/navidrome/navidrome/core/agents"
	"github.com/navidrome/navidrome/core/playlists"
	"github.com/navidrome/navidrome/model"
)

func NewPlaylists(ds model.DataStore, imgUpload playlists.ImageUploadService, ag *agents.Agents) playlists.Playlists {
	playlists.SetArtistSimilarityProvider(ag)
	playlists.SetTrackSimilarityProvider(ag)
	return playlists.NewPlaylists(ds, imgUpload)
}
