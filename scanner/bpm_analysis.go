package scanner

import (
	"context"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/ffmpeg"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
)

type bpmAnalyzer interface {
	AnalyzeBPM(ctx context.Context, filePath string) (int, error)
}

var audioBPMAnalyzer bpmAnalyzer = ffmpeg.New()

func (p *phaseFolders) analyzeBPM(track *model.MediaFile, lib model.Library) {
	if !conf.Server.Scanner.AnalyzeBPM || track.BPM != nil {
		return
	}

	absPath := localAudioPath(lib.Path, track.Path)
	if absPath == "" {
		log.Debug(p.ctx, "Scanner: Skipping BPM analysis for non-local library", "track", track.Path, "library", lib.Name)
		return
	}

	bpm, err := audioBPMAnalyzer.AnalyzeBPM(p.ctx, absPath)
	if err != nil {
		log.Warn(p.ctx, "Scanner: Error analyzing BPM", "track", track.Path, err)
		return
	}
	if bpm <= 0 {
		return
	}
	track.BPM = &bpm
}

func localAudioPath(libPath, trackPath string) string {
	if strings.Contains(libPath, "://") {
		u, err := url.Parse(libPath)
		if err != nil || u.Scheme != "file" {
			return ""
		}
		libPath = u.Path
		if u.Host != "" {
			libPath = filepath.Join(string(filepath.Separator)+u.Host, u.Path)
		}
	}
	return filepath.Join(libPath, trackPath)
}
