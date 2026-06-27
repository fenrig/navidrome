package storage

import (
	"bytes"
	"errors"
	"io"
	"net/url"
	"os"
	"testing"
)

func TestStagedPathLocal(t *testing.T) {
	path, cleanup, err := StagedPath("/music", "Artist/track.mp3")
	if err != nil {
		t.Fatalf("stage local path: %v", err)
	}
	if cleanup != nil {
		t.Fatal("expected no cleanup for local paths")
	}
	if path != "/music/Artist/track.mp3" {
		t.Fatalf("unexpected local path: %q", path)
	}
}

func TestStagedPathRemote(t *testing.T) {
	const schema = "memstage"
	Register(schema, func(_ url.URL) Storage {
		return &fakeStageStorage{data: []byte("hello")}
	})

	path, cleanup, err := StagedPath(schema+"://bucket/prefix", "Artist/track.mp3")
	if err != nil {
		t.Fatalf("stage remote path: %v", err)
	}
	if cleanup == nil {
		t.Fatal("expected cleanup for staged remote file")
	}
	defer func() { _ = cleanup() }()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read staged file: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("unexpected staged contents: %q", string(data))
	}
}

type fakeStageStorage struct {
	data []byte
}

func (s *fakeStageStorage) FS() (MusicFS, error) {
	return nil, errors.New("unused")
}

func (s *fakeStageStorage) Open(_ string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(s.data)), nil
}
