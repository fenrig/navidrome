package scanner

import (
	"context"
	"io"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/ffmpeg"
	"github.com/navidrome/navidrome/core/storage"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
)

type bpmAnalyzer interface {
	AnalyzeBPM(ctx context.Context, filePath string) (int, error)
}

type bpmReaderAnalyzer interface {
	AnalyzeBPMFromReader(ctx context.Context, reader io.Reader) (int, error)
}

var audioBPMAnalyzer bpmAnalyzer = ffmpeg.New()

func (p *phaseFolders) analyzeBPM(track *model.MediaFile, lib model.Library) {
	if !conf.Server.Scanner.AnalyzeBPM || track.BPM != nil {
		return
	}

	if readerAnalyzer, ok := audioBPMAnalyzer.(bpmReaderAnalyzer); ok {
		s, err := storage.For(lib.Path)
		if err != nil {
			log.Warn(p.ctx, "Scanner: Error loading storage for BPM analysis", "track", track.Path, err)
			return
		}
		opener, ok := s.(storage.FileOpener)
		if ok {
			file, err := opener.Open(track.Path)
			if err == nil {
				defer func() { _ = file.Close() }()
				bpm, err := readerAnalyzer.AnalyzeBPMFromReader(p.ctx, file)
				if err != nil {
					log.Warn(p.ctx, "Scanner: Error analyzing BPM", "track", track.Path, err)
					return
				}
				if bpm > 0 {
					track.BPM = &bpm
				}
				return
			}
			log.Warn(p.ctx, "Scanner: Error opening file for BPM analysis", "track", track.Path, err)
		}
	}

	path, cleanup, err := storage.StagedPath(lib.Path, track.Path)
	if err != nil {
		log.Warn(p.ctx, "Scanner: Error staging file for BPM analysis", "track", track.Path, err)
		return
	}
	if cleanup != nil {
		defer func() { _ = cleanup() }()
	}

	bpm, err := audioBPMAnalyzer.AnalyzeBPM(p.ctx, path)
	if err != nil {
		log.Warn(p.ctx, "Scanner: Error analyzing BPM", "track", track.Path, err)
		return
	}
	if bpm <= 0 {
		return
	}
	track.BPM = &bpm
}
