package s3

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"net/url"
	"sort"
	"testing"
	"time"

	"github.com/navidrome/navidrome/model/metadata"
)

func TestParseBucketAndPrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		u      string
		bucket string
		prefix string
	}{
		{name: "host bucket", u: "s3://bucket/music", bucket: "bucket", prefix: "music/"},
		{name: "path bucket", u: "s3:///bucket/music", bucket: "bucket", prefix: "music/"},
		{name: "bucket only", u: "s3://bucket", bucket: "bucket", prefix: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := mustParseURL(t, tt.u)
			bucket, prefix := parseBucketAndPrefix(u)
			if bucket != tt.bucket {
				t.Fatalf("expected bucket %q, got %q", tt.bucket, bucket)
			}
			if prefix != tt.prefix {
				t.Fatalf("expected prefix %q, got %q", tt.prefix, prefix)
			}
		})
	}
}

func TestS3FSOpenAndReadDir(t *testing.T) {
	t.Parallel()

	fsys := newTestFS()

	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		t.Fatalf("read dir root: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "Artist" || !entries[0].IsDir() {
		t.Fatalf("unexpected root entries: %#v", entries)
	}

	dir, err := fsys.Open("Artist/Album")
	if err != nil {
		t.Fatalf("open dir: %v", err)
	}
	defer dir.Close()
	dirEntries, err := dir.(fs.ReadDirFile).ReadDir(-1)
	if err != nil && err != io.EOF {
		t.Fatalf("read dir: %v", err)
	}
	names := []string{dirEntries[0].Name(), dirEntries[1].Name()}
	sort.Strings(names)
	if want := []string{"track1.mp3", "track2.mp3"}; !equalStrings(names, want) {
		t.Fatalf("unexpected album entries: %v", names)
	}

	file, err := fsys.Open("Artist/Album/track1.mp3")
	if err != nil {
		t.Fatalf("open file: %v", err)
	}
	buf := make([]byte, 4)
	n, err := file.Read(buf)
	if err != nil {
		t.Fatalf("read file prefix: %v", err)
	}
	if got := string(buf[:n]); got != "trac" {
		t.Fatalf("unexpected file prefix: %q", got)
	}
	if _, err := file.(io.Seeker).Seek(0, io.SeekStart); err != nil {
		t.Fatalf("seek file: %v", err)
	}
	data, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}
	if string(data) != "track-one" {
		t.Fatalf("unexpected file contents after seek: %q", string(data))
	}
}

func TestS3FSReadTagsAddsFileInfo(t *testing.T) {
	t.Parallel()

	fsys := newTestFS()
	fsys.extractor = fakeExtractor(func(paths ...string) (map[string]metadata.Info, error) {
		out := make(map[string]metadata.Info, len(paths))
		for _, p := range paths {
			out[p] = metadata.Info{}
		}
		return out, nil
	})

	res, err := fsys.ReadTags("Artist/Album/track1.mp3")
	if err != nil {
		t.Fatalf("read tags: %v", err)
	}
	info := res["Artist/Album/track1.mp3"]
	if info.FileInfo == nil {
		t.Fatal("expected file info to be populated")
	}
	if info.FileInfo.Name() != "track1.mp3" {
		t.Fatalf("unexpected file info name: %s", info.FileInfo.Name())
	}
}

func newTestFS() *s3FS {
	tree := newS3Tree("music/")
	now := time.Date(2026, time.June, 27, 12, 0, 0, 0, time.UTC)
	tree.addObject(objectMeta{Key: "music/Artist/Album/track1.mp3", Size: int64(len("track-one")), ModTime: now})
	tree.addObject(objectMeta{Key: "music/Artist/Album/track2.mp3", Size: int64(len("track-two")), ModTime: now.Add(time.Minute)})
	tree.addObject(objectMeta{Key: "music/Artist/cover.jpg", Size: 3, ModTime: now})
	return &s3FS{
		root: tree.root,
		client: &fakeClient{objects: map[string][]byte{
			"music/Artist/Album/track1.mp3": []byte("track-one"),
			"music/Artist/Album/track2.mp3": []byte("track-two"),
			"music/Artist/cover.jpg":        []byte("jpg"),
		}},
		bucket: "bucket",
		prefix: "music/",
	}
}

type fakeClient struct {
	objects map[string][]byte
}

func (c *fakeClient) ListObjects(context.Context, string, string) ([]objectMeta, error) {
	out := make([]objectMeta, 0, len(c.objects))
	for key, data := range c.objects {
		out = append(out, objectMeta{Key: key, Size: int64(len(data)), ModTime: time.Now()})
	}
	return out, nil
}

func (c *fakeClient) GetObject(_ context.Context, _ string, key string) (io.ReadSeekCloser, error) {
	data, ok := c.objects[key]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return &fakeSeekCloser{Reader: bytes.NewReader(data)}, nil
}

type fakeSeekCloser struct {
	*bytes.Reader
}

func (f *fakeSeekCloser) Close() error { return nil }

type fakeExtractor func(paths ...string) (map[string]metadata.Info, error)

func (f fakeExtractor) Parse(paths ...string) (map[string]metadata.Info, error) { return f(paths...) }
func (f fakeExtractor) Version() string                                         { return "fake" }

func mustParseURL(t *testing.T, raw string) (u url.URL) {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return *parsed
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
