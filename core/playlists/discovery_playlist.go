package playlists

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/Masterminds/squirrel"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
)

const discoveryPlaylistCommentMarker = "Auto-generated discovery playlist"

type discoveryPlaylistSpec struct {
	Name         string
	MaxTracks    int
	SeedTracks   int
	MaxPerArtist int
	MaxPerAlbum  int
	MaxPerLabel  int
}

var discoveryPlaylist = discoveryPlaylistSpec{
	Name:         "Discovery Mix",
	MaxTracks:    100,
	SeedTracks:   12,
	MaxPerArtist: 2,
	MaxPerAlbum:  2,
	MaxPerLabel:  3,
}

func (s *playlists) SyncGeneratedDiscoveryPlaylist(ctx context.Context) error {
	admin, err := s.ds.User(ctx).FindFirstAdmin()
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			log.Debug(ctx, "Skipping discovery playlist generation: no admin user found")
			return nil
		}
		return fmt.Errorf("finding admin user: %w", err)
	}

	ctx = request.WithUser(ctx, *admin)

	existing, err := s.ds.Playlist(ctx).GetAll(model.QueryOptions{Sort: "name"})
	if err != nil {
		return fmt.Errorf("loading playlists: %w", err)
	}

	tracks, err := s.ds.MediaFile(ctx).GetAll(model.QueryOptions{Filters: squirrel.Eq{"missing": false}})
	if err != nil {
		return fmt.Errorf("loading media files: %w", err)
	}
	selected := buildDiscoveryMix(tracks, discoveryPlaylist)
	if len(selected) == 0 {
		log.Debug(ctx, "Skipping discovery playlist generation: no suitable tracks")
		return nil
	}

	var target *model.Playlist
	for i := range existing {
		if discoveryPlaylistMatchesManagedSpec(existing[i], admin.ID) {
			target = &existing[i]
			break
		}
	}
	if target == nil {
		target = &model.Playlist{
			Name:    discoveryPlaylist.Name,
			Comment: discoveryPlaylistCommentMarker,
			OwnerID: admin.ID,
			Public:  true,
		}
	}

	target.Name = discoveryPlaylist.Name
	target.Comment = discoveryPlaylistCommentMarker
	target.OwnerID = admin.ID
	target.Public = true
	target.Tracks = nil
	target.AddMediaFiles(selected)

	if err := s.ds.Playlist(ctx).Put(target); err != nil {
		return fmt.Errorf("saving discovery playlist: %w", err)
	}
	if _, err := s.ds.Playlist(ctx).GetWithTracks(target.ID, true, false); err != nil {
		log.Error(ctx, "Error refreshing discovery playlist", "playlist", discoveryPlaylist.Name, err)
	}
	log.Info(ctx, "Synced discovery playlist", "playlist", discoveryPlaylist.Name, "count", len(selected))
	return nil
}

func discoveryPlaylistMatchesManagedSpec(p model.Playlist, adminID string) bool {
	if p.OwnerID != adminID || !p.Public {
		return false
	}
	return p.Comment == discoveryPlaylistCommentMarker
}

func buildDiscoveryMix(tracks model.MediaFiles, spec discoveryPlaylistSpec) model.MediaFiles {
	if len(tracks) == 0 {
		return nil
	}

	affinity := NewTrackAffinity()
	seeds, hasFavorites := discoverySeeds(tracks, spec.SeedTracks)
	if !hasFavorites {
		return buildPopularityMix(tracks, spec)
	}

	selected := make(model.MediaFiles, 0, min(spec.MaxTracks, len(tracks)))
	artistCounts := map[string]int{}
	albumCounts := map[string]int{}
	labelCounts := map[string]int{}
	seen := map[string]struct{}{}
	recent := make(model.MediaFiles, 0, 3)

	first := discoveryStartTrack(tracks)
	if first.ID == "" {
		return buildPopularityMix(tracks, spec)
	}
	selected = append(selected, first)
	seen[first.ID] = struct{}{}
	incrementDiscoveryCounts(first, artistCounts, albumCounts, labelCounts)
	recent = append(recent, first)

	for len(selected) < spec.MaxTracks {
		next, ok := discoveryNextTrack(affinity, tracks, seeds, recent, seen, artistCounts, albumCounts, labelCounts, spec)
		if !ok {
			break
		}
		selected = append(selected, next)
		seen[next.ID] = struct{}{}
		incrementDiscoveryCounts(next, artistCounts, albumCounts, labelCounts)
		recent = append(recent, next)
		if len(recent) > 3 {
			recent = recent[len(recent)-3:]
		}
	}

	return selected
}

func buildPopularityMix(tracks model.MediaFiles, spec discoveryPlaylistSpec) model.MediaFiles {
	type scoredTrack struct {
		track model.MediaFile
		score float64
	}

	scored := make([]scoredTrack, 0, len(tracks))
	for _, track := range tracks {
		score := seedQualityScore(track)
		if score <= 0 {
			score = 1
		}
		scored = append(scored, scoredTrack{track: track, score: score})
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

	selected := make(model.MediaFiles, 0, min(spec.MaxTracks, len(tracks)))
	artistCounts := map[string]int{}
	albumCounts := map[string]int{}
	labelCounts := map[string]int{}
	for _, item := range scored {
		if len(selected) >= spec.MaxTracks {
			break
		}
		if !discoveryAllowsTrack(item.track, artistCounts, albumCounts, labelCounts, spec) {
			continue
		}
		selected = append(selected, item.track)
		incrementDiscoveryCounts(item.track, artistCounts, albumCounts, labelCounts)
	}
	return selected
}

func discoverySeeds(tracks model.MediaFiles, limit int) (model.MediaFiles, bool) {
	type rankedSeed struct {
		track model.MediaFile
		score float64
	}

	seeds := make([]rankedSeed, 0, len(tracks))
	for _, track := range tracks {
		score := seedQualityScore(track)
		if score <= 0 {
			continue
		}
		seeds = append(seeds, rankedSeed{track: track, score: score})
	}
	if len(seeds) == 0 {
		return nil, false
	}

	slices.SortStableFunc(seeds, func(a, b rankedSeed) int {
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

	if limit > len(seeds) {
		limit = len(seeds)
	}
	res := make(model.MediaFiles, limit)
	for i := 0; i < limit; i++ {
		res[i] = seeds[i].track
	}
	return res, true
}

func seedQualityScore(track model.MediaFile) float64 {
	score := float64(track.PlayCount)
	if track.Rating > 0 {
		score += float64(track.Rating * 100)
	}
	if track.Starred {
		score += 1000
	}
	if track.PlayDate != nil {
		score += 10
	}
	return score
}

func discoveryStartTrack(tracks model.MediaFiles) model.MediaFile {
	type scoredTrack struct {
		track model.MediaFile
		score float64
	}

	scored := make([]scoredTrack, 0, len(tracks))
	for _, candidate := range tracks {
		score := seedQualityScore(candidate)
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
		return model.MediaFile{}
	}
	return scored[0].track
}

func discoveryNextTrack(affinity *TrackAffinity, tracks, seeds, recent model.MediaFiles, seen map[string]struct{}, artistCounts, albumCounts, labelCounts map[string]int, spec discoveryPlaylistSpec) (model.MediaFile, bool) {
	type scoredTrack struct {
		track model.MediaFile
		score float64
	}

	scored := make([]scoredTrack, 0, len(tracks))
	for _, candidate := range tracks {
		if _, ok := seen[candidate.ID]; ok {
			continue
		}
		if !discoveryAllowsTrack(candidate, artistCounts, albumCounts, labelCounts, spec) {
			continue
		}
		score := affinity.Score(candidate, seeds, recent)
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
		return model.MediaFile{}, false
	}
	if scored[0].score <= 0 {
		return model.MediaFile{}, false
	}
	return scored[0].track, true
}

func discoveryCandidateScore(candidate model.MediaFile, seeds, recent model.MediaFiles) float64 {
	seedScore := discoveryContextScore(candidate, seeds, []float64{1, 0.82, 0.64}, false)
	recentScore := discoveryContextScore(candidate, recent, []float64{1, 0.85, 0.7}, true)

	score := seedScore*0.8 + recentScore*1.2

	if len(recent) > 0 {
		last := recent[len(recent)-1]
		if sameArtist(candidate, last) {
			score -= 45
			if streak := consecutiveSameArtistCount(candidate, recent); streak > 1 {
				score -= float64((streak - 1) * 80)
			}
		}
		if sameAlbum(candidate, last) {
			score -= 60
		}
		if sameLabel(candidate, last) {
			score -= 8
		}
	}
	return score
}

func discoveryContextScore(candidate model.MediaFile, context model.MediaFiles, weights []float64, newestLast bool) float64 {
	if len(context) == 0 {
		return 0
	}

	best := 0.0
	total := 0.0
	for i := range context {
		weight := 1.0
		if i < len(weights) {
			weight = weights[i]
		}
		track := context[i]
		if newestLast {
			track = context[len(context)-1-i]
		}
		score := seedSimilarityScore(candidate, track)
		if score <= 0 {
			continue
		}
		total += score * weight
		if score > best {
			best = score
		}
	}
	if total == 0 {
		return 0
	}
	return total + best
}

func sameAlbum(a, b model.MediaFile) bool {
	return normalizeKey(a.AlbumID) != "" && normalizeKey(a.AlbumID) == normalizeKey(b.AlbumID)
}

func consecutiveSameArtistCount(candidate model.MediaFile, recent model.MediaFiles) int {
	if len(recent) == 0 {
		return 0
	}

	key := normalizeKey(primaryArtistKey(candidate))
	if key == "" {
		return 0
	}

	count := 0
	for i := len(recent) - 1; i >= 0; i-- {
		if key != normalizeKey(primaryArtistKey(recent[i])) {
			break
		}
		count++
	}
	return count
}

func seedSimilarityScore(candidate, seed model.MediaFile) float64 {
	score := 0.0
	if sameArtist(candidate, seed) {
		score += 60
	}
	if sameAlbumArtist(candidate, seed) {
		score += 30
	}
	if candidate.AlbumID != "" && candidate.AlbumID == seed.AlbumID {
		score += 15
	}
	if genreScore := genreSimilarityScore(candidate, seed); genreScore > 0 {
		score += genreScore
	}
	if sameLabel(candidate, seed) {
		score += 10
	}
	if labelScore := labelSimilarityScore(candidate, seed); labelScore > 0 {
		score += labelScore
	}
	if bpm := bpmCloseness(candidate.BPM, seed.BPM); bpm > 0 {
		score += bpm
	}
	if year := yearCloseness(candidate, seed); year > 0 {
		score += year
	}
	if technical := technicalSimilarityScore(candidate, seed); technical > 0 {
		score += technical
	}
	if external := artistSimilarityBoost(candidate, seed); external > 0 {
		score += external
	}
	if external := trackSimilarityBoost(candidate, seed); external > 0 {
		score += external
	}
	return score
}

func sameArtist(a, b model.MediaFile) bool {
	return normalizeKey(primaryArtistKey(a)) != "" && normalizeKey(primaryArtistKey(a)) == normalizeKey(primaryArtistKey(b))
}

func sameAlbumArtist(a, b model.MediaFile) bool {
	return normalizeKey(primaryAlbumArtistKey(a)) != "" && normalizeKey(primaryAlbumArtistKey(a)) == normalizeKey(primaryAlbumArtistKey(b))
}

func sameLabel(a, b model.MediaFile) bool {
	return normalizeKey(recordLabelKey(a)) != "" && normalizeKey(recordLabelKey(a)) == normalizeKey(recordLabelKey(b))
}

func labelSimilarityScore(a, b model.MediaFile) float64 {
	if aLabel, bLabel := normalizeLabelFamily(recordLabelKey(a)), normalizeLabelFamily(recordLabelKey(b)); aLabel != "" && bLabel != "" && aLabel == bLabel {
		if sameLabel(a, b) {
			return 0
		}
		return 6
	}
	return 0
}

func primaryArtistKey(mf model.MediaFile) string {
	if mf.ArtistID != "" {
		return mf.ArtistID
	}
	if a := mf.Participants.First(model.RoleArtist); a.ID != "" {
		return a.ID
	}
	return mf.Artist
}

func primaryAlbumArtistKey(mf model.MediaFile) string {
	if mf.AlbumArtistID != "" {
		return mf.AlbumArtistID
	}
	if a := mf.Participants.First(model.RoleAlbumArtist); a.ID != "" {
		return a.ID
	}
	return mf.AlbumArtist
}

func recordLabelKey(mf model.MediaFile) string {
	if values := mf.Tags.Values(model.TagRecordLabel); len(values) > 0 {
		return values[0]
	}
	return ""
}

func genreSimilarityScore(a, b model.MediaFile) float64 {
	exactA := exactGenreKeys(a)
	exactB := exactGenreKeys(b)
	exactCount := sharedKeyCount(exactA, exactB)

	familyA := genreFamilyKeys(a)
	familyB := genreFamilyKeys(b)
	familyCount := sharedKeyCount(familyA, familyB)

	sharedFamilies := familyCount - exactCount
	if sharedFamilies < 0 {
		sharedFamilies = 0
	}
	return float64(exactCount)*15 + float64(sharedFamilies)*8
}

func exactGenreKeys(mf model.MediaFile) []string {
	values := mf.Tags.Values(model.TagGenre)
	if len(values) == 0 {
		values = make([]string, 0, len(mf.Genres))
		for _, g := range mf.Genres {
			values = append(values, g.Name)
		}
	}
	return uniqueNormalized(values)
}

func genreFamilyKeys(mf model.MediaFile) []string {
	families := make([]string, 0, len(exactGenreKeys(mf))*3)
	for _, genre := range exactGenreKeys(mf) {
		families = append(families, genreFamilyVariants(genre)...)
	}
	return uniqueNormalized(families)
}

func genreFamilyVariants(genre string) []string {
	genre = normalizeGenreKey(genre)
	if genre == "" {
		return nil
	}

	var out []string
	add := func(values ...string) {
		out = append(out, values...)
	}

	add(genre)
	if strings.Contains(genre, "trance") {
		add("trance")
	}
	if strings.Contains(genre, "psy") && strings.Contains(genre, "trance") {
		add("psytrance")
	}
	if strings.Contains(genre, "goa") && strings.Contains(genre, "trance") {
		add("goa trance")
	}
	if strings.Contains(genre, "downtempo") || strings.Contains(genre, "chill") {
		add("downtempo")
	}
	if strings.Contains(genre, "ambient") {
		add("ambient")
	}
	if strings.Contains(genre, "techno") {
		add("techno")
	}
	if strings.Contains(genre, "house") {
		add("house")
	}
	if strings.Contains(genre, "break") {
		add("breaks")
	}
	if strings.Contains(genre, "drum") && strings.Contains(genre, "bass") {
		add("drum and bass")
	}
	return out
}

func normalizeGenreKey(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	v = strings.NewReplacer("-", " ", "_", " ", "/", " ", "&", " and ").Replace(v)
	v = strings.Join(strings.Fields(v), " ")
	return v
}

func sharedKeyCount(a, b []string) int {
	keys := make(map[string]struct{}, len(a))
	for _, key := range a {
		keys[key] = struct{}{}
	}
	count := 0
	for _, key := range b {
		if _, ok := keys[key]; ok {
			count++
		}
	}
	return count
}

func normalizeLabelFamily(v string) string {
	v = normalizeGenreKey(v)
	if v == "" {
		return ""
	}

	for {
		trimmed := strings.TrimSpace(v)
		switch {
		case strings.HasSuffix(trimmed, " records"):
			v = strings.TrimSpace(strings.TrimSuffix(trimmed, " records"))
		case strings.HasSuffix(trimmed, " record"):
			v = strings.TrimSpace(strings.TrimSuffix(trimmed, " record"))
		case strings.HasSuffix(trimmed, " recordings"):
			v = strings.TrimSpace(strings.TrimSuffix(trimmed, " recordings"))
		case strings.HasSuffix(trimmed, " music"):
			v = strings.TrimSpace(strings.TrimSuffix(trimmed, " music"))
		case strings.HasSuffix(trimmed, " label"):
			v = strings.TrimSpace(strings.TrimSuffix(trimmed, " label"))
		case strings.HasSuffix(trimmed, " productions"):
			v = strings.TrimSpace(strings.TrimSuffix(trimmed, " productions"))
		case strings.HasSuffix(trimmed, " production"):
			v = strings.TrimSpace(strings.TrimSuffix(trimmed, " production"))
		case strings.HasSuffix(trimmed, " entertainment"):
			v = strings.TrimSpace(strings.TrimSuffix(trimmed, " entertainment"))
		case strings.HasSuffix(trimmed, " studios"):
			v = strings.TrimSpace(strings.TrimSuffix(trimmed, " studios"))
		default:
			return trimmed
		}
	}
}

func bpmCloseness(a, b *int) float64 {
	if a == nil || b == nil {
		return 0
	}
	diff := math.Abs(float64(*a - *b))
	switch {
	case diff == 0:
		return 18
	case diff <= 4:
		return 14
	case diff <= 8:
		return 10
	case diff <= 16:
		return 5
	default:
		return 0
	}
}

func yearCloseness(a, b model.MediaFile) float64 {
	ya := bestYear(a)
	yb := bestYear(b)
	if ya == 0 || yb == 0 {
		return 0
	}
	diff := math.Abs(float64(ya - yb))
	switch {
	case diff == 0:
		return 10
	case diff <= 1:
		return 8
	case diff <= 3:
		return 5
	case diff <= 7:
		return 2
	default:
		return 0
	}
}

func technicalSimilarityScore(a, b model.MediaFile) float64 {
	score := 0.0

	switch diff := math.Abs(float64(a.Duration - b.Duration)); {
	case diff == 0:
		score += 6
	case diff <= 15:
		score += 5
	case diff <= 30:
		score += 4
	case diff <= 60:
		score += 2
	}

	switch diff := math.Abs(float64(a.BitRate - b.BitRate)); {
	case a.BitRate > 0 && b.BitRate > 0 && diff == 0:
		score += 3
	case a.BitRate > 0 && b.BitRate > 0 && diff <= 64:
		score += 2
	case a.BitRate > 0 && b.BitRate > 0 && diff <= 128:
		score += 1
	}

	if a.SampleRate > 0 && b.SampleRate > 0 && a.SampleRate == b.SampleRate {
		score += 2
	}
	if a.Channels > 0 && b.Channels > 0 && a.Channels == b.Channels {
		score += 1
	}
	if a.BitDepth != nil && b.BitDepth != nil && *a.BitDepth == *b.BitDepth {
		score += 1
	}
	if a.Codec != "" && b.Codec != "" && strings.EqualFold(a.Codec, b.Codec) {
		score += 1
	}
	if a.TrackNumber > 0 && b.TrackNumber > 0 {
		switch diff := math.Abs(float64(a.TrackNumber - b.TrackNumber)); {
		case diff == 0:
			score += 1
		case diff <= 2:
			score += 0.5
		}
	}
	if a.DiscNumber > 0 && b.DiscNumber > 0 && a.DiscNumber == b.DiscNumber {
		score += 0.5
	}
	return score
}

func artistSimilarityBoost(candidate, seed model.MediaFile) float64 {
	provider := getArtistSimilarityProvider()
	if provider == nil {
		return 0
	}

	candidateKey := normalizeKey(primaryArtistKey(candidate))
	candidateMBID := normalizeKey(candidate.MbzArtistID)
	if candidateKey == "" && candidateMBID == "" {
		return 0
	}

	seedKey := similaritySeedKey(seed)
	if seedKey == "" {
		return 0
	}

	set := getSimilarArtistsForSeed(seed, provider)
	if len(set.byName) == 0 && len(set.byMBID) == 0 {
		return 0
	}
	if candidateMBID != "" {
		if _, ok := set.byMBID[candidateMBID]; ok {
			return 18
		}
	}
	if candidateKey != "" {
		if _, ok := set.byName[candidateKey]; ok {
			return 12
		}
	}
	return 0
}

func trackSimilarityBoost(candidate, seed model.MediaFile) float64 {
	provider := getTrackSimilarityProvider()
	if provider == nil {
		return 0
	}

	candidateKey := normalizeKey(candidate.Title)
	candidateArtist := normalizeKey(primaryArtistName(candidate))
	candidateMBID := normalizeKey(candidate.MbzRecordingID)
	if candidateMBID == "" && candidateKey == "" {
		return 0
	}

	seedKey := similarityTrackSeedKey(seed)
	if seedKey == "" {
		return 0
	}

	set := getSimilarTracksForSeed(seed, provider)
	if len(set.byTitleArtist) == 0 && len(set.byTitle) == 0 && len(set.byMBID) == 0 {
		return 0
	}
	if candidateMBID != "" {
		if _, ok := set.byMBID[candidateMBID]; ok {
			return 20
		}
	}
	if candidateKey != "" && candidateArtist != "" {
		if _, ok := set.byTitleArtist[candidateKey+"|"+candidateArtist]; ok {
			return 14
		}
	}
	if candidateKey != "" {
		if _, ok := set.byTitle[candidateKey]; ok {
			return 8
		}
	}
	return 0
}

func similarityTrackSeedKey(seed model.MediaFile) string {
	if key := normalizeKey(seed.MbzRecordingID); key != "" {
		return "mbid:" + key
	}
	if key := normalizeKey(seed.Title); key != "" {
		return "title:" + key + "|" + normalizeKey(primaryArtistName(seed))
	}
	return ""
}

func getSimilarTracksForSeed(seed model.MediaFile, provider TrackSimilarityProvider) similarTrackSet {
	key := similarityTrackSeedKey(seed)
	if key == "" {
		return similarTrackSet{}
	}

	trackSimilarityCacheMu.Lock()
	if cached, ok := trackSimilarityCache[key]; ok {
		trackSimilarityCacheMu.Unlock()
		return cached
	}
	trackSimilarityCacheMu.Unlock()

	result := similarTrackSet{
		byTitleArtist: map[string]struct{}{},
		byTitle:       map[string]struct{}{},
		byMBID:        map[string]struct{}{},
	}

	ctx := context.Background()
	similar, err := provider.GetSimilarSongsByTrack(ctx, seed.ID, seed.Title, primaryArtistName(seed), seed.MbzRecordingID, 25)
	if err == nil {
		for _, song := range similar {
			if mbid := normalizeKey(song.MBID); mbid != "" {
				result.byMBID[mbid] = struct{}{}
			}
			if title := normalizeKey(song.Name); title != "" {
				result.byTitle[title] = struct{}{}
				if artist := normalizeKey(song.Artist); artist != "" {
					result.byTitleArtist[title+"|"+artist] = struct{}{}
				}
			}
		}
	}

	trackSimilarityCacheMu.Lock()
	trackSimilarityCache[key] = result
	trackSimilarityCacheMu.Unlock()
	return result
}

func primaryArtistName(mf model.MediaFile) string {
	if mf.Artist != "" {
		return mf.Artist
	}
	if a := mf.Participants.First(model.RoleArtist); a.Name != "" {
		return a.Name
	}
	return mf.AlbumArtist
}

func similaritySeedKey(seed model.MediaFile) string {
	if key := normalizeKey(seed.MbzArtistID); key != "" {
		return "mbid:" + key
	}
	if key := normalizeKey(primaryArtistKey(seed)); key != "" {
		return "id:" + key
	}
	if key := normalizeKey(seed.Artist); key != "" {
		return "name:" + key
	}
	return ""
}

func getSimilarArtistsForSeed(seed model.MediaFile, provider ArtistSimilarityProvider) similarArtistSet {
	key := similaritySeedKey(seed)
	if key == "" {
		return similarArtistSet{}
	}

	artistSimilarityCacheMu.Lock()
	if cached, ok := artistSimilarityCache[key]; ok {
		artistSimilarityCacheMu.Unlock()
		return cached
	}
	artistSimilarityCacheMu.Unlock()

	result := similarArtistSet{
		byName: map[string]struct{}{},
		byMBID: map[string]struct{}{},
	}

	ctx := context.Background()
	similar, err := provider.GetSimilarArtists(ctx, seed.ArtistID, seed.Artist, seed.MbzArtistID, 25)
	if err == nil {
		for _, artist := range similar {
			if name := normalizeKey(artist.Name); name != "" {
				result.byName[name] = struct{}{}
			}
			if mbid := normalizeKey(artist.MBID); mbid != "" {
				result.byMBID[mbid] = struct{}{}
			}
		}
	}

	artistSimilarityCacheMu.Lock()
	artistSimilarityCache[key] = result
	artistSimilarityCacheMu.Unlock()
	return result
}

func bestYear(mf model.MediaFile) int {
	for _, y := range []int{mf.ReleaseYear, mf.OriginalYear, mf.Year} {
		if y > 0 {
			return y
		}
	}
	return 0
}

func discoveryAllowsTrack(track model.MediaFile, artistCounts, albumCounts, labelCounts map[string]int, spec discoveryPlaylistSpec) bool {
	artistKey := normalizeKey(primaryArtistKey(track))
	if artistKey != "" && artistCounts[artistKey] >= spec.MaxPerArtist {
		return false
	}
	albumKey := normalizeKey(track.AlbumID)
	if albumKey != "" && albumCounts[albumKey] >= spec.MaxPerAlbum {
		return false
	}
	labelKey := normalizeKey(recordLabelKey(track))
	if labelKey != "" && labelCounts[labelKey] >= spec.MaxPerLabel {
		return false
	}
	return true
}

func incrementDiscoveryCounts(track model.MediaFile, artistCounts, albumCounts, labelCounts map[string]int) {
	if artistKey := normalizeKey(primaryArtistKey(track)); artistKey != "" {
		artistCounts[artistKey]++
	}
	if albumKey := normalizeKey(track.AlbumID); albumKey != "" {
		albumCounts[albumKey]++
	}
	if labelKey := normalizeKey(recordLabelKey(track)); labelKey != "" {
		labelCounts[labelKey]++
	}
}

func normalizeKey(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}

func uniqueNormalized(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = normalizeKey(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func compareFloatDesc(a, b float64) int {
	switch {
	case a > b:
		return -1
	case a < b:
		return 1
	default:
		return 0
	}
}

func compareIntDesc(a, b int) int {
	switch {
	case a > b:
		return -1
	case a < b:
		return 1
	default:
		return 0
	}
}

func compareInt64Desc(a, b int64) int {
	switch {
	case a > b:
		return -1
	case a < b:
		return 1
	default:
		return 0
	}
}

func compareBoolDesc(a, b bool) int {
	switch {
	case a && !b:
		return -1
	case !a && b:
		return 1
	default:
		return 0
	}
}
