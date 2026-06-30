package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/conf/configtest"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/tests"
)

func TestScanMissingBPM(t *testing.T) {
	t.Cleanup(configtest.SetupConfig())
	conf.Server.Scanner.AnalyzeBPM = true

	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "track.mp3"), []byte("fake audio"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	userRepo := tests.CreateMockUserRepo()
	if err := userRepo.Put(&model.User{ID: "admin", UserName: "admin", IsAdmin: true}); err != nil {
		t.Fatalf("put admin: %v", err)
	}

	mediaRepo := tests.CreateMockMediaFileRepo()
	mediaRepo.SetData(model.MediaFiles{
		{
			ID:          "1",
			LibraryID:   1,
			LibraryPath: tempDir,
			Path:        "track.mp3",
			Title:       "Track",
			Artist:      "Artist",
			Album:       "Album",
			Suffix:      "mp3",
		},
		{
			ID:          "2",
			LibraryID:   1,
			LibraryPath: tempDir,
			Path:        "done.mp3",
			Title:       "Done",
			Artist:      "Artist",
			Album:       "Album",
			Suffix:      "mp3",
			BPM:         intPtr(140),
		},
	})

	origAnalyzer := audioBPMAnalyzer
	audioBPMAnalyzer = &fakeBPMAnalyzer{bpm: 132}
	t.Cleanup(func() { audioBPMAnalyzer = origAnalyzer })

	ds := &tests.MockDataStore{
		MockedUser:      userRepo,
		MockedMediaFile: mediaRepo,
	}

	ctx := log.NewContext(context.Background())
	if err := ScanMissingBPM(ctx, ds); err != nil {
		t.Fatalf("scan missing bpm: %v", err)
	}

	got, err := mediaRepo.Get("1")
	if err != nil {
		t.Fatalf("get updated track: %v", err)
	}
	if got.BPM == nil || *got.BPM != 132 {
		t.Fatalf("expected BPM 132, got %#v", got.BPM)
	}
	unchanged, err := mediaRepo.Get("2")
	if err != nil {
		t.Fatalf("get unchanged track: %v", err)
	}
	if unchanged.BPM == nil || *unchanged.BPM != 140 {
		t.Fatalf("expected existing BPM to remain, got %#v", unchanged.BPM)
	}
}

func intPtr(v int) *int { return &v }
