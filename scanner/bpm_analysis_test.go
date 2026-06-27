package scanner

import (
	"context"
	"errors"
	"io"
	"net/url"
	"strings"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/storage"
	"github.com/navidrome/navidrome/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("BPM analysis", func() {
	var analyzer *fakeBPMAnalyzer

	BeforeEach(func() {
		analyzer = &fakeBPMAnalyzer{bpm: 128}
		origAnalyzer := audioBPMAnalyzer
		origAnalyzeBPM := conf.Server.Scanner.AnalyzeBPM
		audioBPMAnalyzer = analyzer
		conf.Server.Scanner.AnalyzeBPM = true
		DeferCleanup(func() {
			audioBPMAnalyzer = origAnalyzer
			conf.Server.Scanner.AnalyzeBPM = origAnalyzeBPM
		})
	})

	It("sets BPM when analysis is enabled and tags did not provide BPM", func() {
		phase := &phaseFolders{ctx: context.Background()}
		track := model.MediaFile{Path: "album/track.flac"}

		phase.analyzeBPM(&track, model.Library{Path: "/music", Name: "Music"})

		Expect(track.BPM).ToNot(BeNil())
		Expect(*track.BPM).To(Equal(128))
		Expect(analyzer.path).To(Equal("/music/album/track.flac"))
	})

	It("does not overwrite BPM from tags", func() {
		phase := &phaseFolders{ctx: context.Background()}
		tagBPM := 140
		track := model.MediaFile{Path: "album/track.flac", BPM: &tagBPM}

		phase.analyzeBPM(&track, model.Library{Path: "/music", Name: "Music"})

		Expect(*track.BPM).To(Equal(140))
		Expect(analyzer.calls).To(Equal(0))
	})

	It("skips non-local libraries", func() {
		phase := &phaseFolders{ctx: context.Background()}
		track := model.MediaFile{Path: "album/track.flac"}

		phase.analyzeBPM(&track, model.Library{Path: "s3://bucket/music", Name: "Remote"})

		Expect(track.BPM).To(BeNil())
		Expect(analyzer.calls).To(Equal(0))
	})

	It("continues when analysis fails", func() {
		analyzer.err = errors.New("no tempo")
		phase := &phaseFolders{ctx: context.Background()}
		track := model.MediaFile{Path: "album/track.flac"}

		phase.analyzeBPM(&track, model.Library{Path: "/music", Name: "Music"})

		Expect(track.BPM).To(BeNil())
		Expect(analyzer.calls).To(Equal(1))
	})

	It("uses a storage reader for remote libraries when available", func() {
		const scheme = "bpmmem"
		storage.Register(scheme, func(_ url.URL) storage.Storage {
			return &fakeBPMStorage{data: []byte("tempo")}
		})

		readerAnalyzer := &fakeReaderBPMAnalyzer{bpm: 132}
		origAnalyzer := audioBPMAnalyzer
		audioBPMAnalyzer = readerAnalyzer
		DeferCleanup(func() {
			audioBPMAnalyzer = origAnalyzer
		})

		phase := &phaseFolders{ctx: context.Background()}
		track := model.MediaFile{Path: "album/track.flac"}

		phase.analyzeBPM(&track, model.Library{Path: scheme + "://bucket/music", Name: "Remote"})

		Expect(track.BPM).ToNot(BeNil())
		Expect(*track.BPM).To(Equal(132))
		Expect(readerAnalyzer.calls).To(Equal(1))
		Expect(readerAnalyzer.data).To(Equal("tempo"))
	})
})

type fakeBPMAnalyzer struct {
	bpm   int
	err   error
	calls int
	path  string
}

func (f *fakeBPMAnalyzer) AnalyzeBPM(_ context.Context, filePath string) (int, error) {
	f.calls++
	f.path = filePath
	return f.bpm, f.err
}

type fakeReaderBPMAnalyzer struct {
	bpm   int
	err   error
	calls int
	data  string
}

func (f *fakeReaderBPMAnalyzer) AnalyzeBPMFromReader(_ context.Context, reader io.Reader) (int, error) {
	f.calls++
	buf, err := io.ReadAll(reader)
	if err != nil {
		return 0, err
	}
	f.data = strings.TrimSpace(string(buf))
	return f.bpm, f.err
}

type fakeBPMStorage struct {
	data []byte
}

func (s *fakeBPMStorage) FS() (storage.MusicFS, error) { return nil, errors.New("unused") }

func (s *fakeBPMStorage) Open(_ string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(string(s.data))), nil
}
