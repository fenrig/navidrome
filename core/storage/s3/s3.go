package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/navidrome/navidrome/adapters/gotaglib"
	"github.com/navidrome/navidrome/core/storage"
	"github.com/navidrome/navidrome/model/metadata"
)

const schema = "s3"

type objectMeta struct {
	Key     string
	Size    int64
	ModTime time.Time
	IsDir   bool
	ETag    string
}

type objectClient interface {
	ListObjects(ctx context.Context, bucket, prefix string) ([]objectMeta, error)
	GetObject(ctx context.Context, bucket, key string) (io.ReadSeekCloser, error)
}

type s3Storage struct {
	u       url.URL
	bucket  string
	prefix  string
	client  objectClient
	initErr error
}

func (s *s3Storage) Open(name string) (io.ReadCloser, error) {
	objectKey := path.Join(s.prefix, normalizeS3Path(name))
	return s.client.GetObject(context.Background(), s.bucket, objectKey)
}

func newS3Storage(u url.URL) storage.Storage {
	bucket, prefix, opts, err := parseS3URL(u)
	if err != nil {
		return &s3Storage{u: u, initErr: err}
	}
	client, err := newMinioClient(opts)
	if err != nil {
		return &s3Storage{u: u, initErr: err}
	}
	return &s3Storage{
		u:      u,
		bucket: bucket,
		prefix: prefix,
		client: realObjectClient{client: client},
	}
}

func (s *s3Storage) FS() (storage.MusicFS, error) {
	if s.initErr != nil {
		return nil, s.initErr
	}
	objects, err := s.client.ListObjects(context.Background(), s.bucket, s.prefix)
	if err != nil {
		return nil, err
	}
	tree := newS3Tree(s.prefix)
	for _, obj := range objects {
		tree.addObject(obj)
	}
	fsys := &s3FS{
		root:   tree.root,
		client: s.client,
		bucket: s.bucket,
		prefix: s.prefix,
	}
	fsys.extractor = gotaglib.NewExtractor(fsys)
	return fsys, nil
}

type realObjectClient struct {
	client *minio.Client
}

func (c realObjectClient) ListObjects(ctx context.Context, bucket, prefix string) ([]objectMeta, error) {
	opts := minio.ListObjectsOptions{Prefix: prefix, Recursive: true}
	out := make([]objectMeta, 0)
	for obj := range c.client.ListObjects(ctx, bucket, opts) {
		if obj.Err != nil {
			return nil, obj.Err
		}
		out = append(out, objectMeta{
			Key:     obj.Key,
			Size:    obj.Size,
			ModTime: obj.LastModified,
			IsDir:   strings.HasSuffix(obj.Key, "/"),
			ETag:    obj.ETag,
		})
	}
	return out, nil
}

func (c realObjectClient) GetObject(ctx context.Context, bucket, key string) (io.ReadSeekCloser, error) {
	return c.client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
}

type s3FS struct {
	root      *s3Node
	client    objectClient
	bucket    string
	prefix    string
	extractor metadataExtractor
}

func (s *s3FS) Stat(name string) (fs.FileInfo, error) {
	n, ok := s.lookup(normalizeS3Path(name))
	if !ok {
		return nil, fs.ErrNotExist
	}
	return n.info(), nil
}

type metadataExtractor interface {
	Parse(files ...string) (map[string]metadata.Info, error)
	Version() string
}

func (s *s3FS) ReadTags(paths ...string) (map[string]metadata.Info, error) {
	if s.extractor == nil {
		return nil, errors.New("no metadata extractor configured")
	}
	res, err := s.extractor.Parse(paths...)
	if err != nil {
		return nil, err
	}
	for p, info := range res {
		if info.FileInfo != nil {
			continue
		}
		stat, err := s.Stat(p)
		if err != nil {
			return nil, err
		}
		fi, ok := stat.(metadata.FileInfo)
		if !ok {
			return nil, fmt.Errorf("filesystem stat for %q does not implement metadata.FileInfo", p)
		}
		info.FileInfo = fi
		res[p] = info
	}
	return res, nil
}

func (s *s3FS) Open(name string) (fs.File, error) {
	n, ok := s.lookup(normalizeS3Path(name))
	if !ok {
		return nil, fs.ErrNotExist
	}
	if n.dir {
		return newS3DirFile(n), nil
	}
	return s.openObject(n)
}

func (s *s3FS) lookup(name string) (*s3Node, bool) {
	if name == "." {
		return s.root, true
	}
	cur := s.root
	for _, part := range strings.Split(name, "/") {
		if part == "" {
			continue
		}
		next, ok := cur.children[part]
		if !ok {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

func (s *s3FS) openObject(n *s3Node) (fs.File, error) {
	reader, err := s.client.GetObject(context.Background(), s.bucket, n.objectKey)
	if err != nil {
		return nil, err
	}
	return &s3ObjectFile{ReadSeekCloser: reader, info: n.info()}, nil
}

func normalizeS3Path(name string) string {
	name = strings.TrimSpace(strings.ReplaceAll(name, "\\", "/"))
	name = strings.TrimPrefix(name, "/")
	if name == "" || name == "." {
		return "."
	}
	name = path.Clean(name)
	if strings.HasPrefix(name, "..") {
		return "."
	}
	return name
}

type s3Tree struct {
	root   *s3Node
	prefix string
}

func newS3Tree(prefix string) *s3Tree {
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return &s3Tree{root: newS3Node(".", true, ""), prefix: prefix}
}

func (t *s3Tree) addObject(obj objectMeta) {
	objectKey := obj.Key
	key := strings.TrimPrefix(objectKey, t.prefix)
	key = strings.TrimPrefix(key, "/")
	if key == "" {
		return
	}
	if strings.HasSuffix(key, "/") && obj.Size == 0 {
		t.ensureDir(strings.TrimSuffix(key, "/"), obj.ModTime)
		return
	}
	parts := strings.Split(key, "/")
	cur := t.root
	for i, part := range parts {
		if part == "" {
			continue
		}
		last := i == len(parts)-1
		child, ok := cur.children[part]
		if !ok {
			child = newS3Node(part, !last, path.Join(cur.path, part))
			child.parent = cur
			cur.children[part] = child
		}
		cur = child
		if obj.ModTime.After(cur.modTime) {
			cur.modTime = obj.ModTime
		}
		if !last {
			cur.dir = true
		}
	}
	cur.dir = false
	cur.objectKey = objectKey
	cur.size = obj.Size
	cur.modTime = obj.ModTime
	for p := cur.parent; p != nil; p = p.parent {
		if obj.ModTime.After(p.modTime) {
			p.modTime = obj.ModTime
		}
	}
}

func (t *s3Tree) ensureDir(dir string, modTime time.Time) {
	if dir == "" || dir == "." {
		return
	}
	parts := strings.Split(dir, "/")
	cur := t.root
	for _, part := range parts {
		if part == "" {
			continue
		}
		child, ok := cur.children[part]
		if !ok {
			child = newS3Node(part, true, path.Join(cur.path, part))
			child.parent = cur
			cur.children[part] = child
		}
		child.dir = true
		if modTime.After(child.modTime) {
			child.modTime = modTime
		}
		cur = child
	}
}

type s3Node struct {
	name      string
	path      string
	objectKey string
	size      int64
	modTime   time.Time
	dir       bool
	parent    *s3Node
	children  map[string]*s3Node
}

func newS3Node(name string, dir bool, fullPath string) *s3Node {
	return &s3Node{
		name:     name,
		path:     fullPath,
		dir:      dir,
		children: make(map[string]*s3Node),
	}
}

func (n *s3Node) info() s3FileInfo {
	return s3FileInfo{
		name:    n.name,
		size:    n.size,
		modTime: n.modTime,
		dir:     n.dir,
	}
}

func (n *s3Node) childEntries() []fs.DirEntry {
	names := make([]string, 0, len(n.children))
	for name := range n.children {
		names = append(names, name)
	}
	slices.Sort(names)
	out := make([]fs.DirEntry, 0, len(names))
	for _, name := range names {
		out = append(out, n.children[name].info())
	}
	return out
}

type s3FileInfo struct {
	name    string
	size    int64
	modTime time.Time
	dir     bool
}

func (fi s3FileInfo) Name() string { return fi.name }
func (fi s3FileInfo) Size() int64  { return fi.size }
func (fi s3FileInfo) Mode() fs.FileMode {
	if fi.dir {
		return fs.ModeDir | 0o555
	}
	return 0o444
}
func (fi s3FileInfo) ModTime() time.Time { return fi.modTime }
func (fi s3FileInfo) IsDir() bool        { return fi.dir }
func (fi s3FileInfo) Sys() any           { return nil }
func (fi s3FileInfo) Type() fs.FileMode {
	if fi.dir {
		return fs.ModeDir
	}
	return 0
}
func (fi s3FileInfo) Info() (fs.FileInfo, error) { return fi, nil }
func (fi s3FileInfo) BirthTime() time.Time       { return fi.modTime }

type s3DirFile struct {
	info    s3FileInfo
	entries []fs.DirEntry
	offset  int
}

func newS3DirFile(n *s3Node) *s3DirFile {
	return &s3DirFile{
		info:    n.info(),
		entries: n.childEntries(),
	}
}

func (f *s3DirFile) Stat() (fs.FileInfo, error) { return f.info, nil }

func (f *s3DirFile) Read([]byte) (int, error) { return 0, io.EOF }

func (f *s3DirFile) Close() error { return nil }

func (f *s3DirFile) ReadDir(n int) ([]fs.DirEntry, error) {
	if f.offset >= len(f.entries) && n > 0 {
		return nil, io.EOF
	}
	if n <= 0 {
		entries := f.entries[f.offset:]
		f.offset = len(f.entries)
		return entries, nil
	}
	end := f.offset + n
	if end > len(f.entries) {
		end = len(f.entries)
	}
	entries := f.entries[f.offset:end]
	f.offset = end
	if f.offset >= len(f.entries) {
		return entries, io.EOF
	}
	return entries, nil
}

type s3ObjectFile struct {
	io.ReadSeekCloser
	info s3FileInfo
}

func (f *s3ObjectFile) Stat() (fs.FileInfo, error) { return f.info, nil }

type minioConfig struct {
	endpoint     string
	region       string
	secure       bool
	pathStyle    bool
	accessKey    string
	secretKey    string
	sessionToken string
	anonymous    bool
}

func parseS3URL(u url.URL) (bucket, prefix string, opts minioConfig, err error) {
	bucket, prefix = parseBucketAndPrefix(u)
	q := u.Query()
	opts.endpoint = q.Get("endpoint")
	opts.region = q.Get("region")
	opts.secure = q.Get("secure") != "false"
	opts.pathStyle = q.Get("pathStyle") == "true"
	opts.accessKey = q.Get("accessKey")
	opts.secretKey = q.Get("secretKey")
	opts.sessionToken = q.Get("sessionToken")
	opts.anonymous = q.Get("anonymous") == "true"
	if opts.endpoint == "" {
		err = fmt.Errorf("s3 storage requires endpoint query parameter")
	}
	return
}

func parseBucketAndPrefix(u url.URL) (bucket, prefix string) {
	host := strings.TrimSpace(u.Host)
	p := strings.Trim(strings.TrimSpace(u.Path), "/")
	switch {
	case host != "":
		bucket = host
		prefix = p
	case p != "":
		parts := strings.Split(p, "/")
		bucket = parts[0]
		if len(parts) > 1 {
			prefix = path.Join(parts[1:]...)
		}
	default:
		bucket = ""
	}
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return
}

func newMinioClient(cfg minioConfig) (*minio.Client, error) {
	var creds *credentials.Credentials
	switch {
	case cfg.anonymous:
		creds = credentials.NewStaticV4("", "", "")
	case cfg.accessKey != "" || cfg.secretKey != "":
		creds = credentials.NewStaticV4(cfg.accessKey, cfg.secretKey, cfg.sessionToken)
	default:
		creds = credentials.NewEnvAWS()
	}
	opts := &minio.Options{
		Creds:  creds,
		Secure: cfg.secure,
		Region: cfg.region,
	}
	if cfg.pathStyle {
		opts.BucketLookup = minio.BucketLookupPath
	}
	return minio.New(cfg.endpoint, opts)
}

func init() {
	storage.Register(schema, newS3Storage)
}
